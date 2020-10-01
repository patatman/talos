// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	machineapi "github.com/talos-systems/talos/pkg/machinery/api/machine"
	"github.com/talos-systems/talos/pkg/machinery/client"
)

const sixMonths = 6 * time.Hour * 24 * 30

var (
	long           bool
	recurse        bool
	recursionDepth int32
	humanizeFlag   bool
)

// lsCmd represents the ls command.
var lsCmd = &cobra.Command{
	Use:     "list [path]",
	Aliases: []string{"ls"},
	Short:   "Retrieve a directory listing",
	Long:    ``,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return WithClient(func(ctx context.Context, c *client.Client) error {
			rootDir := "/"

			if len(args) > 0 {
				rootDir = args[0]
			}

			stream, err := c.LS(ctx, &machineapi.ListRequest{
				Root:           rootDir,
				Recurse:        recurse,
				RecursionDepth: recursionDepth,
			})
			if err != nil {
				return fmt.Errorf("error fetching logs: %s", err)
			}

			defaultNode := client.RemotePeer(stream.Context())

			if !long {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "NODE\tNAME")

				multipleNodes := false
				node := defaultNode

				for {
					info, err := stream.Recv()
					if err != nil {
						if err == io.EOF || status.Code(err) == codes.Canceled {
							if multipleNodes {
								return w.Flush()
							}

							return nil
						}
						return fmt.Errorf("error streaming results: %s", err)
					}

					if info.Metadata != nil && info.Metadata.Hostname != "" {
						multipleNodes = true
						node = info.Metadata.Hostname
					}

					if info.Metadata != nil && info.Metadata.Error != "" {
						fmt.Fprintf(os.Stderr, "%s: %s\n", node, info.Metadata.Error)
						continue
					}

					if info.Error != "" {
						fmt.Fprintf(os.Stderr, "%s: error reading file %s: %s\n", node, info.Name, info.Error)
						continue
					}

					if !multipleNodes {
						fmt.Println(info.RelativeName)
					} else {
						fmt.Fprintf(w, "%s\t%s\n",
							node,
							info.RelativeName,
						)
					}

				}
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NODE\tMODE\tSIZE(B)\tLASTMOD\tNAME")
			for {
				info, err := stream.Recv()
				if err != nil {
					if err == io.EOF || status.Code(err) == codes.Canceled {
						return w.Flush()
					}
					return fmt.Errorf("error streaming results: %s", err)
				}

				node := defaultNode
				if info.Metadata != nil && info.Metadata.Hostname != "" {
					node = info.Metadata.Hostname
				}

				if info.Error != "" {
					fmt.Fprintf(os.Stderr, "%s: error reading file %s: %s\n", node, info.Name, info.Error)
					continue
				}

				if info.Metadata != nil && info.Metadata.Error != "" {
					fmt.Fprintf(os.Stderr, "%s: %s\n", node, info.Metadata.Error)
					continue
				}

				display := info.RelativeName
				if info.Link != "" {
					display += " -> " + info.Link
				}

				size := fmt.Sprintf("%d", info.Size)

				if humanizeFlag {
					size = humanize.Bytes(uint64(info.Size))
				}

				timestamp := time.Unix(info.Modified, 0)
				timestampFormatted := ""

				if humanizeFlag {
					timestampFormatted = humanize.Time(timestamp)
				} else {
					if time.Since(timestamp) < sixMonths {
						timestampFormatted = timestamp.Format("Jan _2 15:04:05")
					} else {
						timestampFormatted = timestamp.Format("Jan _2 2006 15:04")
					}
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					node,
					os.FileMode(info.Mode).String(),
					size,
					timestampFormatted,
					display,
				)
			}
		})
	},
}

func init() {
	lsCmd.Flags().BoolVarP(&long, "long", "l", false, "display additional file details")
	lsCmd.Flags().BoolVarP(&recurse, "recurse", "r", false, "recurse into subdirectories")
	lsCmd.Flags().BoolVarP(&humanizeFlag, "humanize", "H", false, "humanize size and time in the output")
	lsCmd.Flags().Int32VarP(&recursionDepth, "depth", "d", 0, "maximum recursion depth")
	addCommand(lsCmd)
}
