using System.Threading.Tasks;
using System.Windows;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using ArbDesk.Services;

namespace ArbDesk.ViewModels;

public partial class TradingViewModel : ObservableObject
{
    DashboardClient? _client;

    [ObservableProperty] string _broker = "";
    [ObservableProperty] string _symbol = "";
    [ObservableProperty] string _side = "Buy";
    [ObservableProperty] string _lotsText = "0.01";
    [ObservableProperty] string _priceText = "0";
    [ObservableProperty] string _slippageText = "10";
    [ObservableProperty] string _stopLossText = "0";
    [ObservableProperty] string _takeProfitText = "0";

    [ObservableProperty] string _closeBroker = "";
    [ObservableProperty] string _closeTicketText = "0";
    [ObservableProperty] string _closeLotsText = "0";

    [ObservableProperty] string _resultMessage = "";

    public void Initialize(DashboardClient client) => _client = client;

    [RelayCommand]
    async Task SubmitOrder()
    {
        if (_client == null) return;
        if (string.IsNullOrWhiteSpace(Broker) || string.IsNullOrWhiteSpace(Symbol))
        {
            ResultMessage = "Broker and Symbol required";
            return;
        }

        if (!double.TryParse(LotsText, out var lots) || lots <= 0)
        {
            ResultMessage = "Invalid lots";
            return;
        }

        double.TryParse(PriceText, out var price);
        int.TryParse(SlippageText, out var slippage);
        double.TryParse(StopLossText, out var sl);
        double.TryParse(TakeProfitText, out var tp);

        try
        {
            var reply = await _client.SubmitOrderAsync(
                Broker, Symbol, Side, lots, price, slippage, sl, tp);
            ResultMessage = reply.Status == "Filled"
                ? $"Filled: ticket {reply.Ticket}"
                : $"{reply.Status}: {reply.Error}";
        }
        catch (System.Exception ex)
        {
            ResultMessage = $"Error: {ex.Message}";
        }
    }

    [RelayCommand]
    async Task ClosePosition()
    {
        if (_client == null) return;
        if (string.IsNullOrWhiteSpace(CloseBroker) || !long.TryParse(CloseTicketText, out var ticket))
        {
            ResultMessage = "Valid broker and ticket required";
            return;
        }

        double.TryParse(CloseLotsText, out var lots);
        int.TryParse(SlippageText, out var slippage);

        try
        {
            var reply = await _client.ClosePositionAsync(CloseBroker, ticket, lots, slippage);
            ResultMessage = reply.Status == "Closed"
                ? $"Closed: ticket {reply.Ticket}"
                : $"{reply.Status}: {reply.Error}";
        }
        catch (System.Exception ex)
        {
            ResultMessage = $"Error: {ex.Message}";
        }
    }
}
