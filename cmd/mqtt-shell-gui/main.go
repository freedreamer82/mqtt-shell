package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/locale"
	"github.com/freedreamer82/mqtt-shell/internal/pkg/screens"
)

const MainWindowW = 800
const MainWindowH = 450

func main() {

	myApp := app.NewWithID("com.mqtt-shell")
	window := myApp.NewWindow(locale.AppWindowName)

	window.CenterOnScreen()
	//window.SetIcon(resourceLogoSvg)
	window.Resize(fyne.NewSize(MainWindowW, MainWindowH))

	app := screens.NewMainScreen(myApp, window)

	window.SetContent(app.GetContainer())
	window.ShowAndRun()

}
