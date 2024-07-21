package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/agukrapo/go-http-client/client"
	"github.com/agukrapo/playlist-creator/deezer"
	"github.com/agukrapo/playlist-creator/internal/env"
	"github.com/agukrapo/playlist-creator/internal/random"
	"github.com/agukrapo/playlist-creator/internal/set"
	"github.com/agukrapo/playlist-creator/playlists"
)

func main() {
	cookie, err := env.Lookup[string]("DEEZER_ARL_COOKIE")
	if err != nil {
		fyne.LogError("env.Lookup", err)
	}

	app := newApplication(cookie)
	app.ShowAndRun()
}

type application struct {
	window fyne.Window
	form   *widget.Form
	modal  *modal

	cookie string
}

func newApplication(cookie string) *application {
	a := fyneapp.New()
	w := a.NewWindow("Playlist Creator")
	w.Resize(fyne.NewSize(1000, 800))

	return &application{
		window: w,
		cookie: cookie,
		modal:  newModal(w),
	}
}

func (a *application) ShowAndRun() {
	a.renderForm()
	a.window.ShowAndRun()
}

func (a *application) renderForm() {
	arl := widget.NewEntry()
	arl.SetText(a.cookie)
	arl.Validator = notEmpty("ARL")

	name := widget.NewEntry()
	name.SetText("NAME " + random.Name(20))
	name.Validator = notEmpty("name")

	songs := widget.NewMultiLineEntry()
	songs.SetMinRowsVisible(30)
	songs.Validator = notEmpty("songs")

	form := &widget.Form{
		SubmitText: "Search tracks",
		CancelText: "Reset",
		OnCancel: func() {
			arl.SetText(a.cookie)
			name.SetText("NAME " + random.Name(20))
			songs.SetText("")
		},
	}

	form.OnSubmit = func() {
		if err := form.Validate(); err != nil {
			notify(a.window, err)
			return
		}

		a.modal.show()

		target := deezer.New(client.New(), arl.Text)
		a.renderResults(target, name.Text, lines(songs.Text))
	}

	form.Append("ARL", arl)
	form.Append("Name", name)
	form.Append("Songs", songs)

	a.window.SetContent(page("Playlist data", form))
	a.form = form
}

func (a *application) renderResults(target playlists.Target, name string, songs []string) {
	items := make([]*widget.FormItem, 0, len(songs))
	for _, song := range songs {
		items = append(items, &widget.FormItem{
			Text:   song,
			Widget: widget.NewLabel("Searching..."),
		})
	}

	data := set.New(len(songs))
	manager := playlists.NewManager(target, 100)

	cnf := dialog.NewConfirm("Create playlist?", "", func(b bool) {
		if !b {
			return
		}
		s := data.Slice()
		fmt.Println("Creating playlist", name, len(s), s)
		if err := manager.Push(context.Background(), name, s); err != nil {
			notify(a.window, fmt.Errorf("manager.Push: %w", err))
			return
		}
		a.renderForm()
	}, a.window)

	form := &widget.Form{
		Items:      items,
		SubmitText: "Create playlist",
		CancelText: "Back",
		OnCancel: func() {
			a.window.SetContent(page("Playlist data", a.form))
		},
		OnSubmit: func() {
			if data.Length() > 0 {
				cnf.Show()
			}
		},
	}

	if err := manager.Gather(context.Background(), songs, func(i int, _ string, matches []playlists.Track) {
		if len(matches) == 0 {
			items[i].Widget = widget.NewLabel("Not found")
			return
		}

		opts := make([]string, 0, len(matches))
		for _, t := range matches {
			opts = append(opts, t.Name)
		}

		s := widget.NewSelect(opts, nil)
		s.OnChanged = func(v string) {
			si := s.SelectedIndex()
			track := matches[si]
			fmt.Println("selected", si, v, track.ID, track.Name)

			if err := data.Add(i, track.ID, track.Name); err != nil {
				notify(a.window, err)
			}
		}
		s.SetSelectedIndex(0)

		items[i].Widget = s
	}); err != nil {
		notify(a.window, err)
		a.window.SetContent(page("Playlist data", a.form))
	}

	a.window.SetContent(page("Search results", form))
	a.modal.hide()
}

type modal struct {
	window   fyne.Window
	dialog   *dialog.CustomDialog
	activity *widget.Activity
}

func newModal(w fyne.Window) *modal {
	return &modal{
		window: w,
	}
}

func (m *modal) show() {
	prop := canvas.NewRectangle(color.Transparent)
	prop.SetMinSize(fyne.NewSize(50, 50))

	m.activity = widget.NewActivity()
	m.dialog = dialog.NewCustomWithoutButtons("Please wait...", container.NewStack(prop, m.activity), m.window)
	m.activity.Start()
	m.dialog.Show()
}

func (m *modal) hide() {
	m.activity.Stop()
	m.dialog.Hide()
}

func notEmpty(name string) func(v string) error {
	return func(v string) error {
		if v == "" {
			return errors.New("empty " + name)
		}
		return nil
	}
}

func lines(in string) []string {
	var out []string
	for _, line := range strings.Split(in, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func notify(parent fyne.Window, err error) {
	_, _ = fmt.Fprintln(os.Stderr, "Error:", err)
	dialog.ShowError(err, parent)
}

func page(title string, content fyne.CanvasObject) fyne.CanvasObject {
	makeCell := func() fyne.CanvasObject {
		rect := canvas.NewRectangle(nil)
		rect.SetMinSize(fyne.NewSize(10, 10))
		return rect
	}

	label := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	return container.NewBorder(container.NewVBox(label, widget.NewSeparator(), makeCell()), makeCell(), makeCell(), makeCell(), content)
}
