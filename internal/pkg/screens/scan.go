package screens

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/config"
	mqtt "github.com/freedreamer82/mqtt-shell/internal/pkg/mqtt2shell"
)

type OnClientChosen func(c string)

type ScanScreen struct {
	container   fyne.CanvasObject
	clients     []string
	selectedCmd int
	inputcmd    binding.String
	listData    *widget.List
	app         fyne.Window
	mqttOpts    *MQTT.ClientOptions
	clientName  string
	cb          OnClientChosen
}

func (s *ScanScreen) GetContainer() fyne.CanvasObject {
	return s.container
}

func (s *ScanScreen) SetCallbackClient(cb OnClientChosen) {
	s.cb = cb
}

func (s *ScanScreen) GetClientName() string {
	return s.clientName
}

func (s *ScanScreen) GetClients() []string {
	return s.clients
}

func (s *ScanScreen) SetMqttOpts(opt *MQTT.ClientOptions) {
	s.mqttOpts = opt
}

func (s *ScanScreen) Scan() {

	//clear clients list...
	s.clients = []string{}
	discovery := mqtt.NewBeaconDiscovery(s.mqttOpts, config.BeaconRequestTopic, config.BeaconReplyTopic, 5,
		config.BeaconConverter)

	clients := make(chan mqtt.Client, 100)
	quit := make(chan bool)

	go func() {
		for {
			select {
			case client := <-clients:
				fmt.Printf("Ip: %15s - Id: %20s - Version: %10s - Time: %s - Uptime: %s \r\n", client.Ip,
					client.Id, client.Version, client.Time, client.Uptime)
				s.clients = append(s.clients, client.Id)
			case <-quit:
				fmt.Printf("End Scan...")
				return
			}

		}
	}()

	pb := widget.NewProgressBarInfinite()
	pb.Show()
	pop := widget.NewModalPopUp(pb, s.app.Canvas())
	pop.Resize(fyne.NewSize(s.app.Canvas().Size().Width/2, s.app.Canvas().Size().Height/20))
	pop.Show()

	discovery.Run(clients)
	quit <- true

	pb.Refresh()
	pb.Hide()
	pop.Hide()
	s.ShowPopUp()

}

func (s *ScanScreen) ShowPopUp() {
	s.selectedCmd = -1

	cancelButton := widget.NewButton("Cancel", func() {
		s.container.Hide()
	})

	okButton := widget.NewButton("OK", func() {
		s.container.Hide()
		s.clientName = s.clients[s.selectedCmd]
		if s.cb != nil {
			s.cb(s.clientName)
		}
	})
	okButton.Disable()

	s.listData = widget.NewList(
		func() int {
			return len(s.clients)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewLabel(""), layout.NewSpacer())
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*fyne.Container).Objects[0].(*widget.Label).SetText(s.clients[id])
		},
	)
	s.listData.OnSelected = func(id widget.ListItemID) {
		fmt.Printf("selected %d", id)
		//popup.Hide()
		s.selectedCmd = id
		okButton.Enable()
	}

	c := container.NewBorder(nil, container.NewHBox(cancelButton, okButton), nil, nil,
		container.NewScroll(s.listData))

	s.container = widget.NewModalPopUp(c, s.app.Canvas())

	s.container.Resize(fyne.NewSize(s.app.Canvas().Size().Width/2, s.app.Canvas().Size().Height/2))
	s.container.Show()
}

func NewScanOverlay(app fyne.Window, opt *MQTT.ClientOptions) *ScanScreen {

	s := ScanScreen{mqttOpts: opt}
	s.selectedCmd = -1
	s.app = app

	return &s
}
