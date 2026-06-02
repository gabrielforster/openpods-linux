// Command openpods-tray shows AirPods battery in the system tray via a
// StatusNotifierItem (SNI). It reads the daemon's socket and updates the tray
// title/tooltip live, reconnecting if the daemon isn't running yet.
//
// SNI works natively on KDE, GNOME (with the AppIndicator extension), and
// waybar. On i3/polybar (XEmbed) it needs an SNI->XEmbed bridge such as
// snixembed; see docs/linux-bluetooth.md.
package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"
	"time"

	"fyne.io/systray"

	"openpods-linux/assets"
	"openpods-linux/ipc"
	"openpods-linux/render"
)

var socketPath string

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	flag.StringVar(&socketPath, "socket", ipc.DefaultSocketPath(), "daemon socket path")
	flag.Parse()
	systray.Run(onReady, func() {})
}

func onReady() {
	systray.SetIcon(assets.Icon())
	systray.SetTitle("OpenPods")
	systray.SetTooltip("OpenPods — waiting for status…")

	mQuit := systray.AddMenuItem("Quit", "Quit OpenPods")
	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()

	go watchDaemon(socketPath)
}

// trayUI is the slice of the systray API the update logic needs, so it can be
// tested with a fake.
type trayUI interface {
	SetTitle(string)
	SetTooltip(string)
}

type systrayUI struct{}

func (systrayUI) SetTitle(s string)   { systray.SetTitle(s) }
func (systrayUI) SetTooltip(s string) { systray.SetTooltip(s) }

func applySnapshot(ui trayUI, snap ipc.Snapshot) {
	ui.SetTitle(render.Compact(snap))
	ui.SetTooltip(strings.TrimRight(render.Human(snap), "\n"))
}

// watchDaemon connects to the daemon socket and applies each snapshot to the
// tray, reconnecting with a short backoff if the daemon is absent or drops.
func watchDaemon(socket string) {
	ui := systrayUI{}
	for {
		cl, err := ipc.Dial(socket)
		if err != nil {
			ui.SetTitle("OpenPods")
			ui.SetTooltip("OpenPods — daemon not running")
			time.Sleep(3 * time.Second)
			continue
		}
		for {
			snap, err := cl.Read()
			if err != nil {
				cl.Close()
				break
			}
			applySnapshot(ui, snap)
		}
		time.Sleep(2 * time.Second)
	}
}
