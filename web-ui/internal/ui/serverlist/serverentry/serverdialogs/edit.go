// Package serverdialogs holds the dialogs a server card opens: the overview
// with the parameters, the raw .conf viewer, and the editor for the settings
// that can change without recreating the server.

package serverdialogs

import (
	"errors"
	"strconv"
	"strings"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/dialogs"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// ShowEdit opens the editor for an existing server: the endpoint clients dial,
// the resolvers new client configs get, the tunnel MTU and whether the server
// comes up with the container.
//
// None of these existed after creation before: a server created without an
// endpoint (or before detection worked) could only be fixed by editing the
// config on disk, and a VPS whose address changed had no way to say so.
func ShowEdit(e *env.Env, srv api.Server) {
	endpoint := widgets.EntryWithPlaceholder(lang.L("detected public IP"))
	endpoint.SetText(srv.Endpoint)
	dns := widgets.EntryWithText(strings.Join(srv.DNS, ", "))
	mtu := widgets.EntryWithText(strconv.Itoa(srv.MTU))
	autoStart := widgets.NewSwitch(srv.AutoStart, nil)

	content := container.NewVBox(
		widgets.Field(lang.L("Endpoint"), endpoint,
			lang.L("host or address clients connect to; empty follows the detected public IP")),
		widgets.Field(lang.L("DNS"), dns,
			lang.L("comma-separated resolver addresses written into new client configs")),
		widgets.Field(lang.L("MTU"), mtu,
			lang.L("stop the server before changing this: the interface is re-created with it")),
		widgets.ToggleRow(lang.L("Start this server with the container"), autoStart),
	)

	name := map[string]any{"Name": srv.Name}
	dialogs.ShowForm(e.Win, lang.L("Edit server {{.Name}}", name), lang.L("Save"), lang.L("Cancel"),
		content, dialogs.Size(e.Win, 620, 480), func() bool {
			endpointText := strings.TrimSpace(endpoint.Text)
			if endpointText != "" {
				if _, err := api.ParseEndpoint(endpointText); err != nil {
					e.Notify.Fail(errors.New(lang.L("Invalid endpoint: {{.Error}}", map[string]any{"Error": err.Error()})))
					return false
				}
			}

			resolvers := splitResolvers(dns.Text)
			if len(resolvers) == 0 {
				e.Notify.Fail(errors.New(lang.L("At least one DNS server is required")))
				return false
			}
			for _, resolver := range resolvers {
				if _, ok := api.CanonicalIPAddress(resolver); !ok {
					e.Notify.Fail(errors.New(lang.L("Invalid DNS server IP: {{.IP}}", map[string]any{"IP": resolver})))
					return false
				}
			}

			mtuValue, err := strconv.Atoi(strings.TrimSpace(mtu.Text))
			if err != nil || mtuValue < api.MinMTU || mtuValue > api.MaxMTU {
				e.Notify.Fail(errors.New(lang.L("MTU must be between {{.Min}} and {{.Max}}", map[string]any{"Min": api.MinMTU, "Max": api.MaxMTU})))
				return false
			}

			start := autoStart.Checked()
			req := api.UpdateServerRequest{
				Endpoint:  &endpointText,
				DNS:       &resolvers,
				MTU:       &mtuValue,
				AutoStart: &start,
			}

			go func() {
				if _, err := e.Backend.UpdateServer(srv.ID, req); err != nil {
					e.Notify.Fail(err)
					return
				}
				e.Notify.OK(lang.L("Server \"{{.Name}}\" updated", name))
				e.Reload()
			}()
			return true
		})
}

// splitResolvers reads the DNS field the way the create form does.
func splitResolvers(text string) []string {
	var out []string
	for part := range strings.SplitSeq(text, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
