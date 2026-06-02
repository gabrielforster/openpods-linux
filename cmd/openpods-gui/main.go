// Command openpods-gui is the Fyne window frontend: it replicates the Android
// home screen (pod/case artwork + battery + charging/in-ear indicators) and
// updates live from the daemon socket.
package main

import (
	"flag"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"openpods-linux/assets"
	"openpods-linux/ipc"
	"openpods-linux/pods"
	"openpods-linux/render"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	socket := flag.String("socket", ipc.DefaultSocketPath(), "daemon socket path")
	flag.Parse()

	a := app.New()
	a.SetIcon(fyne.NewStaticResource("openpods", assets.Icon()))
	w := a.NewWindow("OpenPods")
	w.SetContent(container.NewCenter(widget.NewLabel("OpenPods — connecting…")))
	w.Resize(fyne.NewSize(380, 260))

	go watch(*socket, w)
	w.ShowAndRun()
}

// watch reads snapshots from the daemon and re-renders the window on the UI
// thread, reconnecting when the daemon is absent or drops.
func watch(socket string, w fyne.Window) {
	for {
		cl, err := ipc.Dial(socket)
		if err != nil {
			fyne.Do(func() { w.SetContent(container.NewCenter(widget.NewLabel("OpenPods — daemon not running"))) })
			time.Sleep(3 * time.Second)
			continue
		}
		for {
			snap, err := cl.Read()
			if err != nil {
				cl.Close()
				break
			}
			content := buildContent(viewModel(snap))
			fyne.Do(func() { w.SetContent(content) })
		}
		time.Sleep(2 * time.Second)
	}
}

// --- view model (pure, testable) ---

type cardData struct {
	Label    string
	Value    string
	Charging bool
	InEar    bool
	Image    []byte
}

type viewData struct {
	Title string
	Cards []cardData
}

func viewModel(s ipc.Snapshot) viewData {
	vd := viewData{Title: render.Name(s)}
	if s.Stale {
		vd.Cards = []cardData{{Value: "updating…"}}
		return vd
	}
	model := pods.ParseModel(s.Model)
	if s.Single {
		vd.Cards = []cardData{card(model, "", assets.Left, s.Left)}
		return vd
	}
	vd.Cards = []cardData{
		card(model, "Left", assets.Left, s.Left),
		card(model, "Right", assets.Right, s.Right),
		card(model, "Case", assets.Case, s.Case),
	}
	return vd
}

func card(m pods.Model, label string, slot assets.Slot, p *ipc.PodView) cardData {
	c := cardData{Label: label, Image: assets.PodImage(m, slot, p != nil)}
	if p == nil {
		c.Value = "—"
		return c
	}
	c.Value = strconv.Itoa(p.Percent) + "%"
	c.Charging = p.Charging
	c.InEar = p.InEar
	return c
}

// --- Fyne widget tree (glue) ---

func buildContent(vd viewData) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(vd.Title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	cards := container.NewHBox()
	for _, c := range vd.Cards {
		cards.Add(buildCard(c))
	}
	// Center the whole thing so it sits in the middle of the window rather than
	// clinging to the top-left (noticeable on large/tiled windows).
	return container.NewCenter(container.NewVBox(title, cards))
}

func buildCard(c cardData) fyne.CanvasObject {
	items := []fyne.CanvasObject{}
	if len(c.Image) > 0 {
		img := canvas.NewImageFromResource(fyne.NewStaticResource(c.Label+"-img", c.Image))
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(96, 96))
		items = append(items, img)
	}
	if c.Label != "" {
		items = append(items, widget.NewLabel(c.Label))
	}
	items = append(items, widget.NewLabelWithStyle(c.Value, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	if ind := indicators(c); ind != "" {
		items = append(items, widget.NewLabelWithStyle(ind, fyne.TextAlignCenter, fyne.TextStyle{Italic: true}))
	}
	return container.NewVBox(items...)
}

func indicators(c cardData) string {
	var parts []string
	if c.Charging {
		parts = append(parts, "⚡ charging")
	}
	if c.InEar {
		parts = append(parts, "in ear")
	}
	return strings.Join(parts, " · ")
}
