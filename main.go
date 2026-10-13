// Command safari-data-tool inspects and manages Safari's WebKit website-data
// storage on macOS. Implementation is built from the spec under
// .kiro/specs/safari-data-tool/.
package main

import (
	"os"

	"github.com/sesamsys/safari-data-tool/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args))
}
