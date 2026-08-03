package desk

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	dashpb "arb/proto/gen/dashboard"
)

// MatrixTab displays the spread matrix in a table.
type MatrixTab struct {
	widget.Table
	client dashpb.DashboardServiceClient
	data   *dashpb.SpreadMatrixReply
	mu     sync.RWMutex
	loaded bool
}

// NewMatrixTab creates a spread matrix tab.
func NewMatrixTab(client dashpb.DashboardServiceClient) fyne.CanvasObject {
	m := &MatrixTab{client: client}
	m.Table = *widget.NewTable(
		func() (int, int) { return m.rows(), m.cols() },
		func() fyne.CanvasObject {
			t := canvas.NewText("", colorTextPrimary)
			t.TextSize = 13
			return t
		},
		m.updateCell,
	)
	m.Table.SetColumnWidth(0, 120)
	go m.streamLoop()
	return m
}

func (m *MatrixTab) rows() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.loaded || m.data == nil || len(m.data.Rows) == 0 {
		return 8
	}
	return len(m.data.Rows) + 1
}

func (m *MatrixTab) cols() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.loaded || m.data == nil || len(m.data.Rows) == 0 {
		return 6
	}
	return int(m.data.TotalSymbols) + 1
}

func (m *MatrixTab) updateCell(id widget.TableCellID, cell fyne.CanvasObject) {
	txt := cell.(*canvas.Text)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.loaded || m.data == nil {
		txt.Text = ""
		txt.Color = colorSkeleton
		return
	}

	if id.Row == 0 {
		if id.Col == 0 {
			txt.Text = "经纪商"
			txt.Color = colorTextDim
			txt.TextStyle = fyne.TextStyle{Bold: true}
		} else if int(id.Col-1) < len(m.data.Rows[0].Cells) {
			txt.Text = m.data.Rows[0].Cells[id.Col-1].Symbol
			txt.Color = colorTextDim
			txt.TextStyle = fyne.TextStyle{Bold: true}
		} else {
			txt.Text = ""
		}
		return
	}

	rowIdx := id.Row - 1
	if rowIdx >= len(m.data.Rows) {
		txt.Text = ""
		return
	}
	row := m.data.Rows[rowIdx]
	if id.Col == 0 {
		txt.Text = row.BrokerName
		txt.Color = colorTextPrimary
		return
	}
	if int(id.Col-1) >= len(row.Cells) {
		txt.Text = ""
		return
	}
	c := row.Cells[id.Col-1]
	if c.IsArbitrageable {
		txt.Color = colorGreen
	} else {
		txt.Color = colorTextPrimary
	}
	txt.Text = fmt.Sprintf("%.1f", c.SpreadToBestAskBps)
}

func (m *MatrixTab) streamLoop() {
	ctx := context.Background()
	for {
		stream, err := m.client.SpreadMatrix(ctx, &dashpb.SpreadMatrixRequest{
			RefreshIntervalMs: 200,
		})
		if err != nil {
			slog.Error("matrix stream", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}
		for {
			reply, err := stream.Recv()
			if err != nil {
				slog.Warn("matrix recv", "error", err)
				time.Sleep(time.Second)
				break
			}
			m.mu.Lock()
			m.data = reply
			m.loaded = true
			m.mu.Unlock()
			fyne.Do(func() { m.Table.Refresh() })
		}
	}
}
