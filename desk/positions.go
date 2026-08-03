package desk

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	dashpb "arb/proto/gen/dashboard"
)

// PositionsTab displays broker positions and account summaries.
type PositionsTab struct {
	widget.Table
	client dashpb.DashboardServiceClient
	data   *dashpb.PositionWatchReply
	mu     sync.RWMutex
	loaded bool
}

// NewPositionsTab creates a positions tab.
func NewPositionsTab(client dashpb.DashboardServiceClient) fyne.CanvasObject {
	p := &PositionsTab{client: client}
	p.Table = *widget.NewTable(
		func() (int, int) { return p.rows(), 7 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		p.updateCell,
	)
	p.Table.SetColumnWidth(0, 140)
	p.Table.SetColumnWidth(1, 100)
	p.Table.SetColumnWidth(2, 80)
	p.Table.SetColumnWidth(3, 60)
	p.Table.SetColumnWidth(4, 60)
	p.Table.SetColumnWidth(5, 100)
	p.Table.SetColumnWidth(6, 120)
	go p.streamLoop()
	return p
}

func (p *PositionsTab) rows() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.loaded || p.data == nil {
		return 6
	}
	count := 1
	for _, bp := range p.data.BrokerPositions {
		count += 1 + len(bp.Positions)
	}
	return count
}

func (p *PositionsTab) updateCell(id widget.TableCellID, cell fyne.CanvasObject) {
	label := cell.(*widget.Label)
	p.mu.RLock()
	defer p.mu.RUnlock()

	headers := []string{"经纪商", "订单号", "品种", "方向", "手数", "净值", "可用保证金"}
	if id.Row == 0 {
		if id.Col < len(headers) {
			label.SetText(headers[id.Col])
		}
		return
	}
	if !p.loaded || p.data == nil {
		label.SetText("")
		return
	}
	rowIdx := 1
	for _, bp := range p.data.BrokerPositions {
		if id.Row == rowIdx {
			switch id.Col {
			case 0:
				label.SetText(bp.BrokerName)
			case 5:
				label.SetText(formatFloat(bp.Equity))
			case 6:
				label.SetText(formatFloat(bp.MarginFree))
			default:
				label.SetText("")
			}
			return
		}
		rowIdx++
		for _, pos := range bp.Positions {
			if id.Row == rowIdx {
				switch id.Col {
				case 1:
					label.SetText(formatInt64(pos.Ticket))
				case 2:
					label.SetText(pos.Symbol)
				case 3:
					label.SetText(pos.Side)
				case 4:
					label.SetText(formatFloat(pos.Lots))
				default:
					label.SetText("")
				}
				return
			}
			rowIdx++
		}
	}
	label.SetText("")
}

func (p *PositionsTab) streamLoop() {
	ctx := context.Background()
	for {
		stream, err := p.client.PositionWatch(ctx, &dashpb.PositionWatchRequest{
			RefreshIntervalMs: 500,
		})
		if err != nil {
			slog.Error("positions stream", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for {
			reply, err := stream.Recv()
			if err != nil {
				slog.Warn("positions recv", "error", err)
				time.Sleep(time.Second)
				break
			}
			p.mu.Lock()
			p.data = reply
			p.loaded = true
			p.mu.Unlock()
			fyne.Do(func() { p.Table.Refresh() })
		}
	}
}
