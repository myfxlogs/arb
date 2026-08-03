package desk

import (
	"context"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	dashpb "arb/proto/gen/dashboard"
)

// AdminTab provides strategy management, circuit breaker reset, and kill switch.
type AdminTab struct {
	client dashpb.DashboardServiceClient
	window fyne.Window
}

// NewAdminTab creates an admin tab.
func NewAdminTab(client dashpb.DashboardServiceClient) fyne.CanvasObject {
	a := &AdminTab{client: client}

	statusLabel := widget.NewLabel("Strategy status: loading...")
	killStatusLabel := widget.NewLabel("Kill switch: unknown")

	refreshBtn := widget.NewButton("Refresh Status", func() {
		a.refreshStatus(statusLabel, killStatusLabel)
	})

	killBtn := widget.NewButton("KILL SWITCH", func() {
		if a.window != nil {
			dialog.ShowConfirm("Kill Switch", "Close all positions and stop trading?", func(ok bool) {
				if ok {
					reply, err := a.client.Kill(context.Background(), &dashpb.KillRequest{})
					if err != nil {
						statusLabel.SetText(fmt.Sprintf("Kill error: %v", err))
						return
					}
					statusLabel.SetText(fmt.Sprintf("Kill: success=%v cancelled=%d",
						reply.Success, reply.OrdersCancelled))
				}
			}, a.window)
		}
	})

	resumeBtn := widget.NewButton("Resume", func() {
		reply, err := a.client.Resume(context.Background(), &dashpb.ResumeRequest{})
		if err != nil {
			statusLabel.SetText(fmt.Sprintf("Resume error: %v", err))
			return
		}
		statusLabel.SetText(fmt.Sprintf("Resume: success=%v", reply.Success))
	})

	resetCBBtn := widget.NewButton("Reset Circuit Breaker", func() {
		reply, err := a.client.ResetGlobalCircuitBreaker(context.Background(), &dashpb.ResetCBRequest{})
		if err != nil {
			statusLabel.SetText(fmt.Sprintf("Reset CB error: %v", err))
			return
		}
		statusLabel.SetText(fmt.Sprintf("CB reset: success=%v", reply.Success))
	})

	return container.NewVBox(
		widget.NewLabel("Admin Control Panel"),
		statusLabel,
		killStatusLabel,
		container.NewHBox(refreshBtn),
		container.NewHBox(killBtn, resumeBtn),
		container.NewHBox(resetCBBtn),
	)
}

func (a *AdminTab) refreshStatus(statusLabel, killStatusLabel *widget.Label) {
	reply, err := a.client.GetStrategyStatus(context.Background(), &dashpb.StrategyStatusRequest{})
	if err != nil {
		statusLabel.SetText(fmt.Sprintf("Error: %v", err))
		return
	}
	text := ""
	for _, item := range reply.Items {
		text += fmt.Sprintf("%s: enabled=%v cb=%v losses=%d pnl=%.2f\n",
			item.Name, item.Enabled, item.CircuitBreakerOpen,
			item.ConsecutiveLosses, item.PnlToday)
	}
	statusLabel.SetText(text)

	ksReply, err := a.client.GetKillSwitchStatus(context.Background(), &dashpb.KillSwitchStatusRequest{})
	if err == nil {
		killStatusLabel.SetText(fmt.Sprintf("Kill switch: active=%v", ksReply.Active))
	}
}
