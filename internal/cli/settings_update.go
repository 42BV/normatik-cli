package cli

import (
	"encoding/json"
	"net/http"

	"github.com/42BV/normatik-cli/internal/api"
	"github.com/42BV/normatik-cli/internal/client"
	"github.com/42BV/normatik-cli/internal/command"
	"github.com/spf13/cobra"
)

const maxSettingsUpdateAttempts = 3

// Only an explicitly rejected stale write is retried. Transport errors may have
// committed, and validation/authorization failures are not concurrency retries.
func updateSettings(read func() (client.SettingsSnapshot, *client.APIError), write func(client.SettingsSnapshot) ([]byte, *client.APIError)) ([]byte, *client.APIError) {
	for attempt := 1; ; attempt++ {
		snapshot, apiErr := read()
		if apiErr != nil {
			return nil, apiErr
		}
		body, apiErr := write(snapshot)
		if apiErr == nil || attempt == maxSettingsUpdateAttempts || apiErr.Status != http.StatusConflict || apiErr.Problem == nil || apiErr.Problem.ErrorCode != "CONCURRENT_UPDATE" {
			return body, apiErr
		}
	}
}

func updateSpaceAppearance(cmd *cobra.Command, d *command.Deps, id int64, name, color, initials string) ([]byte, *client.APIError) {
	return updateSettings(func() (client.SettingsSnapshot, *client.APIError) {
		return d.Client.GetSpaceSettingsSnapshot(cmd.Context(), id)
	}, func(snapshot client.SettingsSnapshot) ([]byte, *client.APIError) {
		var current api.SpaceResult
		if err := json.Unmarshal(snapshot.Body, &current); err != nil {
			return nil, &client.APIError{Malformed: true, Body: snapshot.Body}
		}
		form := api.SpaceAppearanceForm{Name: current.Name, Color: &current.Color, InitialsOverride: current.InitialsOverride}
		if cmd.Flags().Changed("name") {
			form.Name = name
		}
		if cmd.Flags().Changed("color") {
			form.Color = &color
		}
		if cmd.Flags().Changed("initials") {
			form.InitialsOverride = &initials
		}
		return d.Client.UpdateSpace(cmd.Context(), id, form, snapshot.Token)
	})
}

func updateLandingSettings(cmd *cobra.Command, active bool, content string,
	read func() (client.SettingsSnapshot, *client.APIError),
	write func(api.LandingSettingsForm, string) ([]byte, *client.APIError),
) ([]byte, *client.APIError) {
	return updateSettings(read, func(snapshot client.SettingsSnapshot) ([]byte, *client.APIError) {
		form, apiErr := landingForm(cmd, active, content, func() ([]byte, *client.APIError) {
			return snapshot.Body, nil
		})
		if apiErr != nil {
			return nil, apiErr
		}
		return write(form, snapshot.Token)
	})
}
