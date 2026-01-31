// gxx is a shortcut for gx that automatically includes the -y (YOLO mode) flag.
package main

import (
	"os"

	"github.com/nealhardesty/gx/internal/cli"
	"github.com/nealhardesty/gx/internal/version"
)

func main() {
	os.Exit(cli.Run(cli.Options{
		ForceYolo: true,
		Version:   version.Version,
	}))
}
