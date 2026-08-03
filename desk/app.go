package desk

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	dashpb "arb/proto/gen/dashboard"
)

// App is the Fyne desktop application for ARB.
type App struct {
	fyneApp fyne.App
	client  dashpb.DashboardServiceClient
}

// NewApp creates a new desktop application connected to the core gRPC server.
func NewApp(addr string) (*App, error) {
	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial core %s: %w", addr, err)
	}
	client := dashpb.NewDashboardServiceClient(conn)
	return &App{
		fyneApp: app.New(),
		client:  client,
	}, nil
}

// Run starts the Fyne application with 5 tabs.
func (a *App) Run() {
	window := a.fyneApp.NewWindow("ARB Desk")
	tabs := container.NewAppTabs(
		container.NewTabItem("Spread Matrix", NewMatrixTab(a.client)),
		container.NewTabItem("Positions", NewPositionsTab(a.client)),
		container.NewTabItem("Trading", NewTradingTab(a.client)),
		container.NewTabItem("History", NewHistoryTab(a.client)),
		container.NewTabItem("Admin", NewAdminTab(a.client)),
	)
	window.SetContent(tabs)
	window.Resize(fyne.NewSize(1400, 900))
	window.ShowAndRun()
}
