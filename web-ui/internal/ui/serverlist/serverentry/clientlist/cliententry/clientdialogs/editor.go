// Package clientdialogs holds the dialogs a client row opens: the add/edit
// form and the configuration viewer with its QR code.
package clientdialogs

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	datepicker "github.com/sdassow/fyne-datepicker"

	"amneziawg-web-ui/web-ui/api"
	"amneziawg-web-ui/web-ui/internal/ui/dialogs"
	"amneziawg-web-ui/web-ui/internal/ui/env"
	"amneziawg-web-ui/web-ui/internal/ui/testhook"
	"amneziawg-web-ui/web-ui/internal/ui/widgets"
)

// SuspendLayout is how the auto-suspend time is typed and shown: local time,
// to the minute.
const SuspendLayout = "2006-01-02 15:04"

// ShowEditor opens the add/edit form. A nil client means "add new".
func ShowEditor(e *env.Env, server api.Server, client *api.Client) {
	editing := client != nil

	name := widgets.NewEntry()
	testhook.Register("dialog.client.name", name)
	allowedIPs := widgets.EntryWithText("0.0.0.0/0, ::/0")
	suspendAt := widgets.EntryWithPlaceholder(SuspendLayout)

	var iBox fyne.CanvasObject
	applyI := widgets.NewSwitch(false, func(on bool) {
		if iBox == nil {
			return
		}
		if on {
			iBox.Show()
		} else {
			iBox.Hide()
		}
	})
	iEntries := make([]*widget.Entry, 5)
	for i := range iEntries {
		iEntries[i] = widget.NewEntry()
	}

	defaults := e.DefaultI()
	for i, entry := range iEntries {
		key := fmt.Sprintf("i%d", i+1)
		if value := defaults[key]; value != "" {
			entry.SetPlaceHolder(lang.L("server default: {{.Value}}", map[string]any{"Value": widgets.Truncate(value, 40)}))
		} else {
			entry.SetPlaceHolder(lang.L("leave empty to skip"))
		}
	}

	if editing {
		name.SetText(client.Name)
		name.Disable()
		if client.AllowedIPs != "" {
			allowedIPs.SetText(client.AllowedIPs)
		}
		applyI.SetChecked(client.ApplyISettings)
		for i, entry := range iEntries {
			entry.SetText(client.ISettings[fmt.Sprintf("i%d", i+1)])
		}
		if client.SuspendAt != nil {
			suspendAt.SetText(time.Unix(int64(*client.SuspendAt), 0).Local().Format(SuspendLayout))
		}
	} else {
		name.SetPlaceHolder(lang.L("New Client"))
	}

	iGrid := container.NewVBox()
	for i, entry := range iEntries {
		iGrid.Add(widgets.Field(fmt.Sprintf("I%d", i+1), entry, ""))
	}
	iBox = widgets.InnerCard(container.NewVBox(
		container.NewCenter(widgets.SectionTitle(lang.L("Signature packets"))),
		iGrid,
	))
	iNote := widget.NewLabel(lang.L("Each value describes one packet sent before the handshake, as a sequence of tags: " +
		"<b 0x...> static bytes, <t> a 4-byte timestamp, <r N> random bytes, <rc N> random letters, <rd N> random digits " +
		"(N up to 1000). Example: <b 0xd100000001><rc 8><t><r 50>. These are client-only parameters, they are never " +
		"written into the server config; empty values are skipped, and an empty I1 turns the whole set off. Keep each " +
		"packet under about 1472 bytes or it is fragmented on the way out, which is the signature this is meant to avoid. " +
		"A config that grows past the QR code limit can still be downloaded as a file."))
	iNote.Wrapping = fyne.TextWrapWord
	iNote.TextStyle = fyne.TextStyle{Italic: true}
	iGrid.Add(iNote)
	if !applyI.Checked() {
		iBox.Hide()
	}

	fields := []fyne.CanvasObject{
		widgets.Field(lang.L("Client name"), name, ""),
		widgets.Field(lang.L("Allowed IPs"), allowedIPs,
			lang.L("comma-separated ranges routed through the VPN; default 0.0.0.0/0, ::/0")),
	}

	if editing {
		pick := widgets.NewButton("", theme.CalendarIcon(), func() { pickSuspendTime(e, suspendAt) })
		clear := widgets.NewButton("", theme.CancelIcon(), func() { suspendAt.SetText("") })
		fields = append(fields, widgets.Field(lang.L("Auto-suspend at"),
			container.NewBorder(nil, nil, nil, container.NewHBox(pick, clear), suspendAt),
			lang.L("local time, format {{.Layout}}; empty disables auto-suspension", map[string]any{"Layout": SuspendLayout})))
		created := time.Unix(int64(client.CreatedAt), 0).Local().Format("2006-01-02 15:04:05")
		fields = append(fields, widgets.Field(lang.L("Created"), widget.NewLabel(created), ""))
	}

	content := container.NewVBox(
		container.NewVBox(fields...),
		widgets.Separator(),
		widgets.ToggleRow(lang.L("Apply I-settings (custom signature packets I1-I5)"), applyI),
		iBox,
	)

	title := lang.L("Add client to {{.Server}}", map[string]any{"Server": server.Name})
	confirm := lang.L("Add client")
	if editing {
		title = lang.L("Edit client {{.Name}}", map[string]any{"Name": client.Name})
		confirm = lang.L("Save")
	}

	// The add form has two rows and the edit form four plus the auto-suspend
	// field, so the dialog only asks for the height it needs.
	height := float32(430)
	if editing {
		height = 640
	}
	// The fields are clamped to a narrow minimum so a phone can show them; the
	// form itself reads better wider, and the dialog clamps this to the window.
	dialogs.ShowForm(e.Win, title, confirm, lang.L("Cancel"), widgets.AtLeast(520, content), dialogs.Size(e.Win, 720, height), func() bool {
		settings := api.ISettings{}
		var problems []string
		for i, entry := range iEntries {
			value := strings.TrimSpace(entry.Text)
			if value == "" {
				continue
			}
			key := fmt.Sprintf("I%d", i+1)
			if problem := api.ValidateCPS(key, value); problem != "" {
				problems = append(problems, problem)
				continue
			}
			settings[strings.ToLower(key)] = value
		}
		// Only worth checking what will actually be written: with the box
		// unticked the values are kept but never reach a config.
		if applyI.Checked() && len(problems) > 0 {
			e.Notify.Fail(errors.New(strings.Join(problems, "\n")))
			return false
		}

		routes := strings.TrimSpace(allowedIPs.Text)
		if routes == "" {
			routes = "0.0.0.0/0, ::/0"
		}

		if editing {
			save(e, server.ID, *client, routes, applyI.Checked(), settings, strings.TrimSpace(suspendAt.Text))
			return true
		}
		add(e, server, strings.TrimSpace(name.Text), routes, applyI.Checked(), settings)
		return true
	})
}

// pickSuspendTime opens a calendar over the editor and writes the chosen
// moment back into the entry, which stays the one source of truth: typing a
// time by hand keeps working, and the picker starts from whatever the entry
// holds, or from now when it holds nothing parseable.
//
// Only the calendar comes from fyne-datepicker. Its NewDateTimePicker loses
// the hour and minute: they are written into a variable of their own, while
// the confirm callback reads the date picker's, so the time the user set never
// reaches it. The time row is built here instead and joined with the date on
// confirm.
//
// The calendar stays in English in both languages: its "datepicker.*" keys
// sit in the translation files with the English text only because lang.X
// logs a "Translation failure" for every key a language lacks. Translated
// month names would also break it - on the arrows it selects
// time.Month.String(), which is English and matches none of the options.
func pickSuspendTime(e *env.Env, suspendAt *widget.Entry) {
	when := time.Now().Truncate(time.Minute)
	if parsed, err := time.ParseInLocation(SuspendLayout, strings.TrimSpace(suspendAt.Text), time.Local); err == nil {
		when = parsed
	}

	hour := widget.NewSelectEntry(twoDigits(24))
	hour.SetText(when.Format("15"))
	minute := widget.NewSelectEntry(twoDigits(60))
	minute.SetText(when.Format("04"))
	// A select entry asks for the width of its text alone, which clips two
	// digits against the dropdown arrow.
	field := fyne.NewSize(80, hour.MinSize().Height)
	timeRow := container.NewHBox(widget.NewLabel(lang.L("Time")),
		container.NewGridWrap(field, hour), widget.NewLabel(":"), container.NewGridWrap(field, minute))

	var date time.Time
	calendar := datepicker.NewDatePicker(when, time.Monday, func(chosen time.Time, _ bool) { date = chosen })

	content := container.NewBorder(nil, timeRow, nil, nil, calendar)
	dialogs.ShowForm(e.Win, lang.L("Auto-suspend at"), lang.L("OK"), lang.L("Cancel"), content, dialogs.Size(e.Win, 460, 520), func() bool {
		h, errH := strconv.Atoi(strings.TrimSpace(hour.Text))
		m, errM := strconv.Atoi(strings.TrimSpace(minute.Text))
		if errH != nil || errM != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			e.Notify.Fail(errors.New(lang.L("time must be hours 00-23 and minutes 00-59")))
			return false
		}
		calendar.OnActioned(true)
		chosen := time.Date(date.Year(), date.Month(), date.Day(), h, m, 0, 0, time.Local)
		suspendAt.SetText(chosen.Format(SuspendLayout))
		return true
	})
}

// twoDigits lists "00" up to n-1, the options of an hour or minute field.
func twoDigits(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%02d", i)
	}
	return out
}

func add(e *env.Env, server api.Server, name, allowedIPs string, applyI bool, settings api.ISettings) {
	if name == "" {
		name = lang.L("New Client")
	}

	go func() {
		err := e.Backend.AddClient(server.ID, api.AddClientRequest{
			Name:           name,
			ApplyISettings: applyI,
			ISettings:      settings,
			AllowedIPs:     allowedIPs,
		})
		if err != nil {
			e.Notify.Fail(err)
			return
		}
		e.Notify.OK(lang.L("Client \"{{.Name}}\" added", map[string]any{"Name": name}))
		e.Reload()
	}()
}

// save applies the three independent updates the backend exposes for an
// existing peer: routing, I-settings and the auto-suspend timestamp.
func save(e *env.Env, serverID string, client api.Client, allowedIPs string, applyI bool, settings api.ISettings, suspendAt string) {
	var when *time.Time
	if suspendAt != "" {
		parsed, err := time.ParseInLocation(SuspendLayout, suspendAt, time.Local)
		if err != nil {
			e.Notify.Fail(errors.New(lang.L("auto-suspend time must look like {{.Layout}}", map[string]any{"Layout": SuspendLayout})))
			return
		}
		when = &parsed
	}

	go func() {
		if err := e.Backend.UpdateAllowedIPs(serverID, client.ID, allowedIPs); err != nil {
			e.Notify.Fail(err)
			return
		}
		if err := e.Backend.UpdateISettings(serverID, client.ID, applyI, settings); err != nil {
			e.Notify.Fail(err)
			return
		}
		if err := e.Backend.UpdateSuspendTime(serverID, client.ID, when); err != nil {
			e.Notify.Fail(err)
			return
		}
		e.Notify.OK(lang.L("Client \"{{.Name}}\" updated", map[string]any{"Name": client.Name}))
		e.Reload()
	}()
}
