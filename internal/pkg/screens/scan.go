package screens

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/config"
	mqtt "github.com/freedreamer82/mqtt-shell/pkg/mqttchat"
)

type OnClientChosen func(c string)

type ScanScreen struct {
	container   fyne.CanvasObject
	clients     []mqtt.Client
	selectedCmd int
	inputcmd    binding.String
	listData    *widget.List
	app         fyne.Window
	mqttOpts    *MQTT.ClientOptions
	clientName  string
	cb          OnClientChosen
	waitBar     *WaitBar
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

func (s *ScanScreen) GetClients() []mqtt.Client {
	return s.clients
}

func (s *ScanScreen) SetMqttOpts(opt *MQTT.ClientOptions) {
	s.mqttOpts = opt
}

func (s *ScanScreen) Scan() {

	//clear clients list...
	s.clients = []mqtt.Client{}
	discovery := mqtt.NewBeaconDiscovery(s.mqttOpts, config.BeaconRequestTopic, config.BeaconReplyTopic, 5,
		config.BeaconConverter)

	clients := make(chan mqtt.Client, 100)
	quit := make(chan bool)

	go func() {
		for {
			select {
			case client := <-clients:
				//fmt.Printf("Ip: %15s - Id: %20s - Version: %10s - Time: %s - Uptime: %s \r\n", client.Ip,
				//	client.Id, client.Version, client.Time, client.Uptime)
				s.clients = append(s.clients, client)
			case <-quit:
				fmt.Printf("End Scan...")
				return
			}

		}
	}()

	s.waitBar.Resize(fyne.NewSize(s.app.Canvas().Size().Width/2, s.app.Canvas().Size().Height/20))
	s.waitBar.Show()

	discovery.Run(clients)
	quit <- true

	s.waitBar.Hide()
	s.ShowPopUp()

}

func (s *ScanScreen) ShowPopUp() {
	s.selectedCmd = -1

	cancelButton := widget.NewButton("Cancel", func() {
		s.container.Hide()
	})

	okButton := widget.NewButton("OK", func() {
		s.container.Hide()
		s.clientName = s.clients[s.selectedCmd].Id
		if s.cb != nil {
			s.cb(s.clientName)
		}
	})
	okButton.Disable()

	infoButton := widget.NewButton("info", func() {

		msg := []string{}
		msg = append(msg, "Name: "+s.clients[s.selectedCmd].Id)
		msg = append(msg, "Version: "+s.clients[s.selectedCmd].Version)
		msg = append(msg, "Ip: "+s.clients[s.selectedCmd].Ip)
		msg = append(msg, "Uptime: "+s.clients[s.selectedCmd].Time)

		dialog.ShowInformation("Info", strings.Join(msg, "\n"), s.app)
	})
	infoButton.Disable()

	s.listData = widget.NewList(
		func() int {
			return len(s.clients)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(widget.NewLabel(""), layout.NewSpacer(), widget.NewLabel(""), widget.NewLabel(""))
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			item.(*fyne.Container).Objects[0].(*widget.Label).SetText(s.clients[id].Id)
			//item.(*fyne.Container).Objects[2].(*widget.Label).SetText(s.clients[id].Ip)
			item.(*fyne.Container).Objects[3].(*widget.Label).SetText(s.clients[id].Version)
		},
	)
	s.listData.OnSelected = func(id widget.ListItemID) {
		fmt.Printf("selected %d", id)
		s.selectedCmd = id
		okButton.Enable()
		infoButton.Enable()
	}

	c := container.NewBorder(nil, container.NewHBox(cancelButton, okButton, layout.NewSpacer(), infoButton), nil, nil,
		container.NewScroll(s.listData))

	s.container = widget.NewModalPopUp(c, s.app.Canvas())

	w := s.app.Canvas().Size().Width / 2
	if fyne.CurrentDevice().IsMobile() {
		w = s.app.Canvas().Size().Width
	}
	s.container.Resize(fyne.NewSize(w, s.app.Canvas().Size().Height/2))
	s.container.Show()
}

func NewScanOverlay(app fyne.Window, opt *MQTT.ClientOptions) *ScanScreen {

	s := ScanScreen{mqttOpts: opt}

	s.waitBar = NewWaitBar(app)
	s.selectedCmd = -1
	s.app = app

	return &s
}
