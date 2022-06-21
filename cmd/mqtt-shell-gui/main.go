package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/bundle"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/constant"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/locale"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/screens"
)

func main() {

	myApp := app.NewWithID(constant.APP_ID)
	window := myApp.NewWindow(locale.AppWindowName)

	window.CenterOnScreen()
	window.SetIcon(bundle.ResourceMqttShellMidResolutionPng)
	window.Resize(fyne.NewSize(constant.MainWindowW, constant.MainWindowH))

	app := screens.NewMainScreen(myApp, window)

	window.SetContent(app.GetContainer())

	window.ShowAndRun()

}
