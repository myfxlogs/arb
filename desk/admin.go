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

	statusLabel := widget.NewLabel("策略状态: 加载中...")
	killStatusLabel := widget.NewLabel("紧急停止: 未知")

	refreshBtn := widget.NewButton("刷新状态", func() {
		a.refreshStatus(statusLabel, killStatusLabel)
	})

	killBtn := widget.NewButton("紧急停止", func() {
		if a.window != nil {
			dialog.ShowConfirm("紧急停止", "确认平仓所有持仓并停止交易？", func(ok bool) {
				if ok {
					reply, err := a.client.Kill(context.Background(), &dashpb.KillRequest{})
					if err != nil {
						statusLabel.SetText(fmt.Sprintf("停止错误: %v", err))
						return
					}
					statusLabel.SetText(fmt.Sprintf("停止: 成功=%v 取消订单=%d",
						reply.Success, reply.OrdersCancelled))
				}
			}, a.window)
		}
	})

	resumeBtn := widget.NewButton("恢复交易", func() {
		reply, err := a.client.Resume(context.Background(), &dashpb.ResumeRequest{})
		if err != nil {
			statusLabel.SetText(fmt.Sprintf("恢复错误: %v", err))
			return
		}
		statusLabel.SetText(fmt.Sprintf("恢复: 成功=%v", reply.Success))
	})

	resetCBBtn := widget.NewButton("重置熔断器", func() {
		reply, err := a.client.ResetGlobalCircuitBreaker(context.Background(), &dashpb.ResetCBRequest{})
		if err != nil {
			statusLabel.SetText(fmt.Sprintf("重置熔断器错误: %v", err))
			return
		}
		statusLabel.SetText(fmt.Sprintf("熔断器重置: 成功=%v", reply.Success))
	})

	return container.NewVBox(
		widget.NewLabel("管理控制面板"),
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
		statusLabel.SetText(fmt.Sprintf("错误: %v", err))
		return
	}
	text := ""
	for _, item := range reply.Items {
		text += fmt.Sprintf("%s: 启用=%v 熔断=%v 连亏=%d 今日盈亏=%.2f\n",
			item.Name, item.Enabled, item.CircuitBreakerOpen,
			item.ConsecutiveLosses, item.PnlToday)
	}
	statusLabel.SetText(text)

	ksReply, err := a.client.GetKillSwitchStatus(context.Background(), &dashpb.KillSwitchStatusRequest{})
	if err == nil {
		killStatusLabel.SetText(fmt.Sprintf("紧急停止: 已激活=%v", ksReply.Active))
	}
}
