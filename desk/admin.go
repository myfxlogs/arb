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

// AdminTab provides strategy management, circuit breaker reset, kill switch, and broker management.
type AdminTab struct {
	client     dashpb.DashboardServiceClient
	window     fyne.Window
	brokerList *widget.List
	brokers    []string
	selectedBroker int
}

// NewAdminTab creates an admin tab.
func NewAdminTab(client dashpb.DashboardServiceClient, win fyne.Window) fyne.CanvasObject {
	a := &AdminTab{client: client, window: win}

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

	// === 经纪商管理 ===
	a.brokerList = widget.NewList(
		func() int { return len(a.brokers) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, obj fyne.CanvasObject) {
			if i < len(a.brokers) {
				obj.(*widget.Label).SetText(a.brokers[i])
			}
		},
	)
	a.selectedBroker = -1
	a.brokerList.OnSelected = func(id widget.ListItemID) {
		a.selectedBroker = int(id)
	}

	addBrokerBtn := widget.NewButton("添加经纪商", func() {
		a.showAddBrokerDialog()
	})

	removeBrokerBtn := widget.NewButton("删除经纪商", func() {
		if a.selectedBroker < 0 || a.selectedBroker >= len(a.brokers) {
			return
		}
		name := a.brokers[a.selectedBroker]
		if a.window != nil {
			dialog.ShowConfirm("删除经纪商", fmt.Sprintf("确认删除经纪商 %s？", name), func(ok bool) {
				if ok {
					reply, err := a.client.RemoveBroker(context.Background(), &dashpb.RemoveBrokerRequest{Name: name})
					if err != nil {
						dialog.ShowError(err, a.window)
						return
					}
					if !reply.Success {
						dialog.ShowInformation("错误", reply.Error, a.window)
						return
					}
					a.refreshBrokers()
				}
			}, a.window)
		}
	})

	refreshBrokerBtn := widget.NewButton("刷新列表", func() {
		a.refreshBrokers()
	})

	brokerSection := container.NewBorder(
		container.NewVBox(
			widget.NewLabel("经纪商管理"),
			container.NewHBox(addBrokerBtn, removeBrokerBtn, refreshBrokerBtn),
		),
		nil, nil, nil,
		a.brokerList,
	)

	riskSection := container.NewVBox(
		widget.NewLabel("风控管理"),
		statusLabel,
		killStatusLabel,
		container.NewHBox(refreshBtn),
		container.NewHBox(killBtn, resumeBtn),
		container.NewHBox(resetCBBtn),
	)

	go a.refreshBrokers()

	return container.NewVBox(
		widget.NewLabel("管理控制面板"),
		riskSection,
		widget.NewSeparator(),
		brokerSection,
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

func (a *AdminTab) refreshBrokers() {
	reply, err := a.client.GetAccountSnapshots(context.Background(), &dashpb.AccountSnapshotRequest{})
	if err != nil {
		return
	}
	a.brokers = make([]string, 0, len(reply.Items))
	for _, item := range reply.Items {
		a.brokers = append(a.brokers, item.BrokerName)
	}
	a.brokerList.Refresh()
}

func (a *AdminTab) showAddBrokerDialog() {
	if a.window == nil {
		return
	}

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("经纪商名称（如 OctaFX-Demo）")

	platformSelect := widget.NewSelect([]string{"MT4", "MT5"}, nil)
	platformSelect.SetSelected("MT5")

	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder("服务器地址（如 78.140.180.198）")

	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("端口（如 443）")

	userEntry := widget.NewEntry()
	userEntry.SetPlaceHolder("交易账号")

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("密码")

	form := container.NewVBox(
		widget.NewLabel("名称"), nameEntry,
		widget.NewLabel("平台"), platformSelect,
		widget.NewLabel("服务器地址"), hostEntry,
		widget.NewLabel("端口"), portEntry,
		widget.NewLabel("账号"), userEntry,
		widget.NewLabel("密码"), passwordEntry,
	)

	dialog.ShowCustomConfirm("添加经纪商", "添加", "取消", form, func(confirmed bool) {
		if !confirmed {
			return
		}
		platform := int32(0)
		if platformSelect.Selected == "MT5" {
			platform = 1
		}
		req := &dashpb.AddBrokerRequest{
			Name:     nameEntry.Text,
			Platform: platform,
			Host:     hostEntry.Text,
			Port:     int32(parseInt(portEntry.Text)),
			User:     int64(parseInt(userEntry.Text)),
			Password: passwordEntry.Text,
		}
		reply, err := a.client.AddBroker(context.Background(), req)
		if err != nil {
			dialog.ShowInformation("错误", fmt.Sprintf("错误: %v", err), a.window)
			return
		}
		if !reply.Success {
			dialog.ShowInformation("失败", reply.Error, a.window)
			return
		}
		dialog.ShowInformation("成功", fmt.Sprintf("经纪商 %s 添加成功", nameEntry.Text), a.window)
		a.refreshBrokers()
	}, a.window)
}
