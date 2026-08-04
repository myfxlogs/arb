package main

import (
	"flag"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"arb/desk"
	"arb/frontend"
)

func main() {
	addr := flag.String("addr", "qalfa.org:443", "core gRPC address")
	flag.Parse()

	deskApp, err := desk.NewApp(*addr)
	if err != nil {
		log.Fatalf("create desk app: %v", err)
	}

	app := application.New(application.Options{
		Name:        "ARB 交易终端",
		Description: "跨平台跨经纪商套利系统",
		Services: []application.Service{
			application.NewService(deskApp),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontend.Assets),
		},
	})

	deskApp.SetApp(app)

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "ARB 交易终端",
		Width:  1400,
		Height: 900,
	})

	if err := app.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
