using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Linq;
using System.Runtime.CompilerServices;
using ArbDesk.Dashboard;

namespace ArbDesk.ViewModels;

public sealed class OpportunityRow : INotifyPropertyChanged
{
    public string Id { get; }
    public string Type { get; private set; }
    public string CanonicalSymbol { get; private set; }
    public string LegsCompact { get; private set; }
    public string NetBps { get; private set; }
    public string NetProfit { get; private set; }
    public string GrossProfit { get; private set; }
    public string AnnualizedNetBps { get; private set; }
    public string Confidence { get; private set; }
    public string ExpiresAt { get; private set; }
    public bool Executable { get; private set; }
    public string RejectReason { get; private set; }

    string _status = "Pushed";
    public string Status
    {
        get => _status;
        set { _status = value; OnPropertyChanged(); OnPropertyChanged(nameof(CanConfirm)); }
    }

    public bool CanConfirm => Status == "Pushed" && Executable;

    public IReadOnlyList<LegRow> Legs { get; private set; }

    public OpportunityRow(Opportunity opp)
    {
        Id = opp.Id;
        UpdateFrom(opp);
    }

    public void UpdateFrom(Opportunity opp)
    {
        Type = opp.Type switch
        {
            OppType.CrossExchange => "CrossEx",
            OppType.Carry => "Carry",
            OppType.Triangular => "Tri",
            _ => "?"
        };

        CanonicalSymbol = opp.Legs.Count > 0 ? opp.Legs[0].CanonicalSymbol : "";
        LegsCompact = string.Join(" | ", opp.Legs.Select(l =>
            $"{l.Broker} {l.Direction} {l.Lots}"));
        NetBps = opp.NetBps;
        NetProfit = opp.NetProfit;
        GrossProfit = opp.GrossProfit;
        AnnualizedNetBps = opp.AnnualizedNetBps;
        Confidence = $"{opp.Confidence:F2}";
        ExpiresAt = DateTimeOffset.FromUnixTimeMilliseconds(
            opp.ExpiresAtUnixMs).LocalDateTime.ToString("HH:mm:ss");
        Executable = opp.Executable;
        RejectReason = opp.RejectReason;
        Legs = opp.Legs.Select(l => new LegRow(l)).ToList();

        OnPropertyChanged(string.Empty);
    }

    public event PropertyChangedEventHandler? PropertyChanged;
    void OnPropertyChanged([CallerMemberName] string? name = null) =>
        PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));
}

public sealed class LegRow
{
    public string Broker { get; }
    public string BrokerSymbol { get; }
    public string CanonicalSymbol { get; }
    public string Direction { get; }
    public string Lots { get; }
    public string EstimatePrice { get; }
    public string Role { get; }
    public string DailySwap { get; }
    public string AnnualizedBps { get; }

    public LegRow(Leg leg)
    {
        Broker = leg.Broker;
        BrokerSymbol = leg.BrokerSymbol;
        CanonicalSymbol = leg.CanonicalSymbol;
        Direction = leg.Direction switch
        {
            BuySell.Buy => "Buy",
            BuySell.Sell => "Sell",
            _ => "-"
        };
        Lots = leg.Lots;
        EstimatePrice = leg.EstimatePrice;
        Role = leg.Role switch
        {
            LegRole.Income => "Income",
            LegRole.Hedge => "Hedge",
            _ => ""
        };
        DailySwap = leg.DailySwap;
        AnnualizedBps = leg.AnnualizedBps;
    }
}
