// mcskel is a starter template for new CLIs: copy the entire cli-skeleton/
// directory, globally replace the mcskel/cli-skeleton identifiers, and start
// building. New CLIs must not re-implement output envelopes, fingerprints,
// credential storage, or password reading — always import the shared packages.
package main

import (
	"os"

	"github.com/x0c/agent-friendly-cli-skill/cli-skeleton/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
