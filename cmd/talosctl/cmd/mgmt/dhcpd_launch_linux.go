// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package mgmt

import (
	"net"

	"github.com/spf13/cobra"

	"github.com/talos-systems/talos/pkg/provision/providers/vm"
)

var dhcpdLaunchCmdFlags struct {
	addr      string
	ifName    string
	statePath string
}

// dhcpdLaunchCmd represents the loadbalancer-launch command.
var dhcpdLaunchCmd = &cobra.Command{
	Use:    "dhcpd-launch",
	Short:  "Internal command used by VM provisioners",
	Long:   ``,
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return vm.DHCPd(dhcpdLaunchCmdFlags.ifName, net.ParseIP(dhcpdLaunchCmdFlags.addr), dhcpdLaunchCmdFlags.statePath)
	},
}

func init() {
	dhcpdLaunchCmd.Flags().StringVar(&dhcpdLaunchCmdFlags.addr, "addr", "localhost", "IP address to listen on")
	dhcpdLaunchCmd.Flags().StringVar(&dhcpdLaunchCmdFlags.ifName, "interface", "", "interface to listen on")
	dhcpdLaunchCmd.Flags().StringVar(&dhcpdLaunchCmdFlags.statePath, "state-path", "", "path to state directory")
	addCommand(dhcpdLaunchCmd)
}
