package desk

import (
	"crypto/tls"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
	var opts []grpc.DialOption
	if isLocal(addr) {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds := credentials.NewTLS(&tls.Config{
			ServerName: hostFromAddr(addr),
		})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	}
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial core %s: %w", addr, err)
	}
	client := dashpb.NewDashboardServiceClient(conn)
	return &App{
		fyneApp: app.New(),
		client:  client,
	}, nil
}

func isLocal(addr string) bool {
	return strings.HasPrefix(addr, "localhost") || strings.HasPrefix(addr, "127.")
}

func hostFromAddr(addr string) string {
	host, _, ok := strings.Cut(addr, ":")
	if !ok {
		return addr
	}
	return host
}

// Run starts the Fyne application with 5 tabs.
func (a *App) Run() {
	window := a.fyneApp.NewWindow("ARB 交易终端")
	tabs := container.NewAppTabs(
		container.NewTabItem("价差矩阵", NewMatrixTab(a.client)),
		container.NewTabItem("持仓", NewPositionsTab(a.client)),
		container.NewTabItem("交易", NewTradingTab(a.client)),
		container.NewTabItem("历史", NewHistoryTab(a.client)),
		container.NewTabItem("管理", NewAdminTab(a.client, window)),
	)
	window.SetContent(tabs)
	window.Resize(fyne.NewSize(1400, 900))
	window.ShowAndRun()
}
