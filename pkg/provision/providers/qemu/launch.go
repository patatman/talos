// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package qemu

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/google/uuid"

	"github.com/talos-systems/talos/pkg/blockdevice/table/gpt"
	"github.com/talos-systems/talos/pkg/provision"
	"github.com/talos-systems/talos/pkg/provision/internal/cniutils"
	"github.com/talos-systems/talos/pkg/provision/providers/vm"
)

// LaunchConfig is passed in to the Launch function over stdin.
type LaunchConfig struct {
	StatePath string

	// VM options
	DiskPath          string
	VCPUCount         int64
	MemSize           int64
	QemuExecutable    string
	KernelImagePath   string
	InitrdPath        string
	PFlashImages      []string
	KernelArgs        string
	MachineType       string
	EnableKVM         bool
	BootloaderEnabled bool
	NodeUUID          uuid.UUID

	// Talos config
	Config string

	// Network
	NetworkConfig *libcni.NetworkConfigList
	CNI           provision.CNIConfig
	IP            net.IP
	CIDR          net.IPNet
	Hostname      string
	GatewayAddr   net.IP
	MTU           int
	Nameservers   []net.IP

	// PXE
	TFTPServer       string
	BootFilename     string
	IPXEBootFileName string

	// API
	APIPort int

	// filled by CNI invocation
	tapName string
	vmMAC   string
	ns      ns.NetNS

	// signals
	c chan os.Signal

	// controller
	controller *Controller
}

// withCNI creates network namespace, launches CNI and passes control to the next function
// filling config with netNS and interface details.
func withCNI(ctx context.Context, config *LaunchConfig, f func(config *LaunchConfig) error) error {
	// random ID for the CNI, maps to single VM
	containerID := uuid.New().String()

	cniConfig := libcni.NewCNIConfigWithCacheDir(config.CNI.BinPath, config.CNI.CacheDir, nil)

	// create a network namespace
	ns, err := testutils.NewNS()
	if err != nil {
		return err
	}

	defer func() {
		ns.Close()              //nolint: errcheck
		testutils.UnmountNS(ns) //nolint: errcheck
	}()

	ones, _ := config.CIDR.Mask.Size()
	runtimeConf := libcni.RuntimeConf{
		ContainerID: containerID,
		NetNS:       ns.Path(),
		IfName:      "veth0",
		Args: [][2]string{
			{"IP", fmt.Sprintf("%s/%d", config.IP, ones)},
			{"GATEWAY", config.GatewayAddr.String()},
		},
	}

	// attempt to clean up network in case it was deployed previously
	err = cniConfig.DelNetworkList(ctx, config.NetworkConfig, &runtimeConf)
	if err != nil {
		return fmt.Errorf("error deleting CNI network: %w", err)
	}

	res, err := cniConfig.AddNetworkList(ctx, config.NetworkConfig, &runtimeConf)
	if err != nil {
		return fmt.Errorf("error provisioning CNI network: %w", err)
	}

	defer func() {
		if e := cniConfig.DelNetworkList(ctx, config.NetworkConfig, &runtimeConf); e != nil {
			log.Printf("error cleaning up CNI: %s", e)
		}
	}()

	currentResult, err := current.NewResultFromResult(res)
	if err != nil {
		return fmt.Errorf("failed to parse cni result: %w", err)
	}

	vmIface, tapIface, err := cniutils.VMTapPair(currentResult, containerID)
	if err != nil {
		return fmt.Errorf(
			"failed to parse VM network configuration from CNI output, ensure CNI is configured with a plugin " +
				"that supports automatic VM network configuration such as tc-redirect-tap",
		)
	}

	config.tapName = tapIface.Name
	config.vmMAC = vmIface.Mac
	config.ns = ns

	// dump node IP/mac/hostname for dhcp
	if err = vm.DumpIPAMRecord(config.StatePath, vm.IPAMRecord{
		IP:               config.IP,
		Netmask:          config.CIDR.Mask,
		MAC:              vmIface.Mac,
		Hostname:         config.Hostname,
		Gateway:          config.GatewayAddr,
		MTU:              config.MTU,
		Nameservers:      config.Nameservers,
		TFTPServer:       config.TFTPServer,
		IPXEBootFilename: config.IPXEBootFileName,
	}); err != nil {
		return err
	}

	return f(config)
}

func checkPartitions(config *LaunchConfig) (bool, error) {
	disk, err := os.Open(config.DiskPath)
	if err != nil {
		return false, fmt.Errorf("failed to open disk file %w", err)
	}

	defer disk.Close() //nolint: errcheck

	diskTable, err := gpt.NewGPT("vda", disk)
	if err != nil {
		return false, fmt.Errorf("error creating GPT object: %w", err)
	}

	if err = diskTable.Read(); err != nil {
		return false, nil
	}

	return len(diskTable.Partitions()) > 0, nil
}

// launchVM runs qemu with args built based on config.
//
//nolint: gocyclo
func launchVM(config *LaunchConfig) error {
	bootOrder := "cn"

	if config.controller.ForcePXEBoot() {
		bootOrder = "nc"
	}

	args := []string{
		"-m", strconv.FormatInt(config.MemSize, 10),
		"-drive", fmt.Sprintf("format=raw,if=virtio,file=%s", config.DiskPath),
		"-smp", fmt.Sprintf("cpus=%d", config.VCPUCount),
		"-cpu", "max",
		"-nographic",
		"-netdev", fmt.Sprintf("tap,id=net0,ifname=%s,script=no,downscript=no", config.tapName),
		"-device", fmt.Sprintf("virtio-net-pci,netdev=net0,mac=%s", config.vmMAC),
		"-device", "virtio-rng-pci",
		"-no-reboot",
		"-boot", fmt.Sprintf("order=%s,reboot-timeout=5000", bootOrder),
		"-smbios", fmt.Sprintf("type=1,uuid=%s", config.NodeUUID),
	}

	machineArg := config.MachineType

	if config.EnableKVM {
		machineArg += ",accel=kvm"
	}

	args = append(args, "-machine", machineArg)

	pflashArgs := make([]string, 2*len(config.PFlashImages))
	for i := range config.PFlashImages {
		pflashArgs[2*i] = "-drive"
		pflashArgs[2*i+1] = fmt.Sprintf("file=%s,format=raw,if=pflash", config.PFlashImages[i])
	}

	args = append(args, pflashArgs...)

	// check if disk is empty/wiped
	diskBootable, err := checkPartitions(config)
	if err != nil {
		return err
	}

	if (!diskBootable || !config.BootloaderEnabled) && config.KernelImagePath != "" {
		args = append(args,
			"-kernel", config.KernelImagePath,
			"-initrd", config.InitrdPath,
			"-append", config.KernelArgs,
		)
	}

	fmt.Fprintf(os.Stderr, "starting qemu with args:\n%s\n", strings.Join(args, " "))
	cmd := exec.Command(
		config.QemuExecutable,
		args...,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := ns.WithNetNSPath(config.ns.Path(), func(_ ns.NetNS) error {
		return cmd.Start()
	}); err != nil {
		return err
	}

	done := make(chan error)

	go func() {
		done <- cmd.Wait()
	}()

	for {
		select {
		case sig := <-config.c:
			fmt.Fprintf(os.Stderr, "exiting VM as signal %s was received\n", sig)

			if err := cmd.Process.Kill(); err != nil {
				return fmt.Errorf("failed to kill process %w", err)
			}

			return fmt.Errorf("process stopped")
		case err := <-done:
			if err != nil {
				return fmt.Errorf("process exited with error %s", err)
			}

			// graceful exit
			return nil
		case command := <-config.controller.CommandsCh():
			if command == VMCommandStop {
				fmt.Fprintf(os.Stderr, "exiting VM as stop command via API was received\n")

				if err := cmd.Process.Kill(); err != nil {
					return fmt.Errorf("failed to kill process %w", err)
				}

				<-done

				return nil
			}
		}
	}
}

// Launch a control process around qemu VM manager.
//
// This function is invoked from 'talosctl qemu-launch' hidden command
// and wraps starting, controlling 'qemu' VM process.
//
// Launch restarts VM forever until control process is stopped itself with a signal.
//
// Process is expected to receive configuration on stdin. Current working directory
// should be cluster state directory, process output should be redirected to the
// logfile in state directory.
//
// When signals SIGINT, SIGTERM are received, control process stops qemu and exits.
//
//nolint: gocyclo
func Launch() error {
	var config LaunchConfig

	ctx := context.Background()

	if err := vm.ReadConfig(&config); err != nil {
		return err
	}

	config.c = vm.ConfigureSignals()
	config.controller = NewController()

	httpServer, err := vm.NewHTTPServer(config.GatewayAddr, config.APIPort, []byte(config.Config), config.controller)
	if err != nil {
		return err
	}

	httpServer.Serve()
	defer httpServer.Shutdown(ctx) //nolint: errcheck

	// patch kernel args
	config.KernelArgs = strings.ReplaceAll(config.KernelArgs, "{TALOS_CONFIG_URL}", fmt.Sprintf("http://%s/config.yaml", httpServer.GetAddr()))

	return withCNI(ctx, &config, func(config *LaunchConfig) error {
		for {
			for config.controller.PowerState() != PoweredOn {
				select {
				case <-config.controller.CommandsCh():
					// machine might have been powered on
				case sig := <-config.c:
					fmt.Fprintf(os.Stderr, "exiting VM as signal %s was received\n", sig)

					return fmt.Errorf("process stopped")
				}
			}

			if err := launchVM(config); err != nil {
				return err
			}
		}
	})
}
