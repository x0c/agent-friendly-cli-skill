// The doctor command demonstrates a standard health check: local credentials
// plus server reachability.
package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/x0c/agent-friendly-cli-skill/cli-common/appframe"
	"github.com/x0c/agent-friendly-cli-skill/cli-common/output"
)

func newDoctorCommand() *cobra.Command {
	var name string
	command := &cobra.Command{Use: "doctor", Short: "Check local credentials and server reachability", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		p, err := loadProfile(name)
		if err != nil {
			return err
		}
		client, err := withSessionClient(p)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		req, err := client.NewRequest(ctx, http.MethodGet, "/", nil, "")
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		reachable := resp.StatusCode < 500
		data := map[string]any{
			"name": p.Name, "server": p.Server,
			"reachable": reachable, "logged_in": p.LoggedIn(),
		}
		human := fmt.Sprintf("Profile %s: %s", p.Name, map[bool]string{true: "server reachable", false: "server unreachable"}[reachable])
		return appframe.Respond(cmd, data, human, output.Meta{})
	}}
	command.Flags().StringVar(&name, "name", "", "Profile name")
	return command
}
