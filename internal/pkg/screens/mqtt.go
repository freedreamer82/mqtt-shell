package screens

import (
	"errors"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	mqtt "github.com/freedreamer82/mqtt-shell/internal/pkg/mqtt2shell"
)

type MqttDialog struct {
	container   fyne.CanvasObject
	dialog      dialog.Dialog
	isConnected bool
	mqttClient  MQTT.Client
	mqttOpts    *MQTT.ClientOptions
	scanScreen  *ScanScreen
	app         fyne.Window
	storage     fyne.Preferences
	user        *widget.Entry
	broker      *widget.Entry
	password    *widget.Entry
	port        *widget.Entry
}

const mqttBroker = "mqttBroker"
const mqttBrokerPassword = "mqttBrokerPassword"
const mqttBrokerUser = "mqttBrokerUser"
const mqttBrokerHost = "mqttBrokerHost"
const mqttBrokerPort = "mqttBrokerPort"

func (s *MqttDialog) GetContainer() fyne.CanvasObject {
	return s.container
}

func (s *MqttDialog) connectionCb(status mqtt.ConnectionStatus) {
	if status == mqtt.ConnectionStatus_Connected {
		s.isConnected = true

	} else if status == mqtt.ConnectionStatus_Disconnected {
		s.isConnected = false
	}
}

func (s *MqttDialog) saveDataToStorage() {

}

func (s *MqttDialog) getDataFromStorage() {

}

func (s *MqttDialog) createForm() {

	s.broker.SetPlaceHolder("broker host")
	s.broker.Validator = func(br string) error {
		if br == "" {
			return errors.New("password not correct")
		}

		return nil
	}
	if text := s.storage.String(mqttBroker); text != "" {
		s.broker.SetText(text)
	}

	s.user.SetPlaceHolder("user")
	if text := s.storage.String(mqttBrokerUser); text != "" {
		s.user.SetText(text)
	}

	s.port.SetPlaceHolder("1883")
	if text := s.storage.String(mqttBrokerPort); text != "" {
		s.port.SetText(text)
	}

	s.password.SetPlaceHolder("")
	if text := s.storage.String(mqttBrokerPassword); text != "" {
		s.password.SetText(text)
	}

	s.dialog = dialog.NewForm("Mqtt broker settings", "Connect", "Cancel",
		[]*widget.FormItem{
			{Text: "Broker", Widget: s.broker, HintText: "MQTT broker to connect to"},
			{Text: "Port", Widget: s.port, HintText: "MQTT broker port"},
			{Text: "User", Widget: s.user, HintText: "User to use for connecting (optional)"},
			{Text: "Password", Widget: s.password, HintText: "User password to use for connecting (optional)"},
		},
		func(confirm bool) {
			if !confirm {

				defer s.createForm()

				return
			}

			brokerurl := s.broker.Text
			addr := fmt.Sprintf("tcp://%s:%s", brokerurl, s.port.Text)

			opts := MQTT.NewClientOptions()
			opts.AddBroker(addr)
			if s.user.Text != "" {
				opts.SetUsername(s.user.Text)
			}
			if s.password.Text != "" {
				opts.SetPassword(s.password.Text)
			}
			opts.AutoReconnect = true
			s.mqttOpts = opts
			s.mqttClient = MQTT.NewClient(s.mqttOpts)
			s.scanScreen.SetMqttOpts(s.mqttOpts)
			s.scanScreen.Scan()

			if len(s.scanScreen.GetClients()) != 0 {
				s.storage.SetString(mqttBrokerPort, s.port.Text)
				s.storage.SetString(mqttBrokerUser, s.user.Text)
				s.storage.SetString(mqttBrokerPassword, s.password.Text)
				s.storage.SetString(mqttBroker, s.broker.Text)
			}

		}, s.app)
	s.dialog.Resize(fyne.NewSize(s.app.Canvas().Size().Width/2, s.app.Canvas().Size().Height/2))
	s.dialog.Show()

}

func NewMqttDialog(app fyne.Window, storage fyne.Preferences) *MqttDialog {
	w := widget.NewLabel("")

	s := MqttDialog{container: w, app: app, storage: storage}
	s.scanScreen = NewScanOverlay(app, s.mqttOpts)
	s.user = widget.NewEntry()
	s.broker = widget.NewEntry()
	s.password = widget.NewPasswordEntry()
	s.port = widget.NewEntry()

	s.createForm()
	s.dialog.Resize(fyne.NewSize(400, 100))
	//s.dialog.Show()

	return &s
}
