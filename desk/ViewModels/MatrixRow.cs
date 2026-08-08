using System.Collections.Generic;
using System.Linq;
using ArbDesk.Dashboard;

namespace ArbDesk.ViewModels;

public sealed class MatrixRow
{
    public string Broker { get; }
    public string BaseCurrency { get; }
    public string SwapLongBps { get; }
    public string SwapShortBps { get; }
    public bool IsConnected { get; }
    public string FreeMargin { get; }
    public string Equity { get; }
    public IReadOnlyList<MatrixCell> Cells { get; }

    public string SymbolsSummary =>
        Cells.Count > 0 ? $"{Cells.Count} symbols" : "";

    public MatrixRow(SpreadMatrixReply.Types.BrokerRow row)
    {
        Broker = row.BrokerName;
        BaseCurrency = row.BaseCurrency;
        SwapLongBps = $"{row.DailySwapLongBps:F1}";
        SwapShortBps = $"{row.DailySwapShortBps:F1}";
        IsConnected = row.IsConnected;
        FreeMargin = $"{row.FreeMargin:F2}";
        Equity = $"{row.Equity:F2}";
        Cells = row.Cells.Select(c => new MatrixCell(c)).ToList();
    }
}

public sealed class MatrixCell
{
    public string Symbol { get; }
    public string Bid { get; }
    public string Ask { get; }
    public string SpreadToBestAskBps { get; }
    public string SpreadToBestBidBps { get; }
    public bool IsArbitrageable { get; }
    public string NetProfitBps { get; }

    public MatrixCell(SpreadMatrixReply.Types.SpreadCell cell)
    {
        Symbol = cell.Symbol;
        Bid = $"{cell.Bid:F5}";
        Ask = $"{cell.Ask:F5}";
        SpreadToBestAskBps = $"{cell.SpreadToBestAskBps:F1}";
        SpreadToBestBidBps = $"{cell.SpreadToBestBidBps:F1}";
        IsArbitrageable = cell.IsArbitrageable;
        NetProfitBps = $"{cell.EstimatedNetProfitBps:F1}";
    }
}
