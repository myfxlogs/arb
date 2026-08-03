package desk

import (
	"context"
	"fmt"
	"strings"

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

	// 平台选择
	platformSelect := widget.NewSelect([]string{"MT4", "MT5"}, nil)
	platformSelect.SetSelected("MT5")

	// 搜索框
	companyEntry := widget.NewEntry()
	companyEntry.SetPlaceHolder("输入经纪商名称关键词（如 OctaFX, RoboForex）")

	searchStatus := widget.NewLabel("")
	companySelect := widget.NewSelect([]string{}, nil)
	serverSelect := widget.NewSelect([]string{}, nil)

	// 搜索结果缓存
	var searchResult *dashpb.SearchBrokerReply

	// 服务器选择变化时，显示服务器地址
	serverInfo := widget.NewLabel("")

	// 账号密码
	userEntry := widget.NewEntry()
	userEntry.SetPlaceHolder("交易账号")
	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("密码")

	// 自定义名称
	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder("自定义名称（可留空，默认用公司名）")

	searchBtn := widget.NewButton("搜索", func() {
		platform := int32(0)
		if platformSelect.Selected == "MT5" {
			platform = 1
		}
		searchStatus.SetText("搜索中...")
		companySelect.Options = []string{}
		serverSelect.Options = []string{}
		serverInfo.SetText("")
		companySelect.ClearSelected()
		serverSelect.ClearSelected()

		go func() {
			reply, err := a.client.SearchBroker(context.Background(), &dashpb.SearchBrokerRequest{
				Company:  companyEntry.Text,
				Platform: platform,
			})
			fyne.Do(func() {
				if err != nil {
					searchStatus.SetText(fmt.Sprintf("搜索失败: %v", err))
					return
				}
				if reply.Error != "" {
					searchStatus.SetText(reply.Error)
					return
				}
				searchResult = reply
				companies := make([]string, 0, len(reply.Companies))
				for _, c := range reply.Companies {
					companies = append(companies, c.CompanyName)
				}
				if len(companies) == 0 {
					searchStatus.SetText("未找到匹配的经纪商")
					return
				}
				companySelect.Options = companies
				companySelect.SetSelectedIndex(0)
				searchStatus.SetText(fmt.Sprintf("找到 %d 个经纪商", len(companies)))
			})
		}()
	})

	// 公司选择变化时，更新服务器列表
	companySelect.OnChanged = func(companyName string) {
		if searchResult == nil {
			return
		}
		for _, c := range searchResult.Companies {
			if c.CompanyName == companyName {
				servers := make([]string, 0, len(c.Servers))
				for _, s := range c.Servers {
					label := s.Name
					if s.Access != "" {
						label = fmt.Sprintf("%s (%s)", s.Name, s.Access)
					}
					servers = append(servers, label)
				}
				serverSelect.Options = servers
				if len(servers) > 0 {
					serverSelect.SetSelectedIndex(0)
				}
				return
			}
		}
	}

	// 服务器选择变化时，显示地址信息
	serverSelect.OnChanged = func(serverLabel string) {
		if searchResult == nil {
			return
		}
		for _, c := range searchResult.Companies {
			if c.CompanyName != companySelect.Selected {
				continue
			}
			for _, s := range c.Servers {
				label := s.Name
				if s.Access != "" {
					label = fmt.Sprintf("%s (%s)", s.Name, s.Access)
				}
				if label == serverLabel {
					serverInfo.SetText(fmt.Sprintf("服务器: %s\n地址: %s", s.Name, s.Access))
					return
				}
			}
		}
	}

	form := container.NewVBox(
		widget.NewLabel("平台"), platformSelect,
		widget.NewLabel("经纪商名称"), companyEntry,
		searchBtn,
		searchStatus,
		widget.NewLabel("选择公司"), companySelect,
		widget.NewLabel("选择服务器"), serverSelect,
		serverInfo,
		widget.NewSeparator(),
		widget.NewLabel("交易账号"), userEntry,
		widget.NewLabel("密码"), passwordEntry,
		widget.NewLabel("自定义名称（可选）"), nameEntry,
	)

	dialog.ShowCustomConfirm("添加经纪商", "添加", "取消", form, func(confirmed bool) {
		if !confirmed {
			return
		}
		platform := int32(0)
		if platformSelect.Selected == "MT5" {
			platform = 1
		}

		// 解析服务器信息
		serverName := ""
		host := ""
		port := int32(443)
		if searchResult != nil {
			for _, c := range searchResult.Companies {
				if c.CompanyName != companySelect.Selected {
					continue
				}
				for _, s := range c.Servers {
					label := s.Name
					if s.Access != "" {
						label = fmt.Sprintf("%s (%s)", s.Name, s.Access)
					}
					if label == serverSelect.Selected {
						serverName = s.Name
						if s.Access != "" {
							parts := strings.Split(s.Access, ":")
							host = parts[0]
							if len(parts) > 1 {
								port = int32(parseInt(parts[1]))
							}
						}
						break
					}
				}
			}
		}

		if host == "" && serverName == "" {
			dialog.ShowInformation("错误", "请先搜索并选择经纪商和服务器", a.window)
			return
		}
		if userEntry.Text == "" {
			dialog.ShowInformation("错误", "请输入交易账号", a.window)
			return
		}
		if passwordEntry.Text == "" {
			dialog.ShowInformation("错误", "请输入密码", a.window)
			return
		}

		name := nameEntry.Text
		if name == "" {
			name = companySelect.Selected
		}

		req := &dashpb.AddBrokerRequest{
			Name:     name,
			Platform: platform,
			Host:     host,
			Port:     port,
			User:     int64(parseInt(userEntry.Text)),
			Password: passwordEntry.Text,
			Server:   serverName,
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
		dialog.ShowInformation("成功", fmt.Sprintf("经纪商 %s 添加成功", name), a.window)
		a.refreshBrokers()
	}, a.window)
}
