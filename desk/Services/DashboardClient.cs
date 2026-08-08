using Grpc.Net.Client;
using ArbDesk.Dashboard;

namespace ArbDesk.Services;

public sealed class DashboardClient : IDisposable
{
    readonly GrpcChannel _channel;
    readonly DashboardService.DashboardServiceClient _client;

    public DashboardClient(string address = "http://localhost:50051")
    {
        _channel = GrpcChannel.ForAddress(address);
        _client = new DashboardService.DashboardServiceClient(_channel);
    }

    public IAsyncEnumerable<OpportunityEvent> OpportunityStream(CancellationToken ct = default) =>
        _client.OpportunityStream(new OpportunityStreamRequest()).ResponseStream.ReadAllAsync(ct);

    public IAsyncEnumerable<SpreadMatrixReply> SpreadMatrixStream(CancellationToken ct = default) =>
        _client.SpreadMatrix(new SpreadMatrixRequest()).ResponseStream.ReadAllAsync(ct);

    public IAsyncEnumerable<PositionWatchReply> PositionWatchStream(CancellationToken ct = default) =>
        _client.PositionWatch(new PositionWatchRequest()).ResponseStream.ReadAllAsync(ct);

    public async Task<ConfirmReply> ConfirmOpportunityAsync(string id) =>
        await _client.ConfirmOpportunityAsync(new ConfirmRequest { OpportunityId = id });

    public async Task<ManualOrderReply> SubmitOrderAsync(string broker, string symbol,
        string side, double lots, double price = 0, int slippage = 0,
        double stopLoss = 0, double takeProfit = 0) =>
        await _client.SubmitOrderAsync(new ManualOrderRequest
        {
            BrokerName = broker, Symbol = symbol, Side = side, Lots = lots,
            Price = price, Slippage = slippage, StopLoss = stopLoss, TakeProfit = takeProfit
        });

    public async Task<ClosePositionReply> ClosePositionAsync(
        string broker, long ticket, double lots = 0, int slippage = 0) =>
        await _client.ClosePositionAsync(new ClosePositionRequest
        {
            BrokerName = broker, Ticket = ticket, Lots = lots, Slippage = slippage
        });

    public async Task<SignalHistoryReply> GetSignalHistoryAsync(
        long fromMs, long toMs, string strategy = "", int limit = 100) =>
        await _client.GetSignalHistoryAsync(new SignalHistoryRequest
        {
            FromUnixMs = fromMs, ToUnixMs = toMs, Strategy = strategy, Limit = limit
        });

    public async Task<KillReply> KillAsync() =>
        await _client.KillAsync(new KillRequest());

    public async Task<ResumeReply> ResumeAsync() =>
        await _client.ResumeAsync(new ResumeRequest());

    public async Task<StrategyStatusReply> GetStrategyStatusAsync() =>
        await _client.GetStrategyStatusAsync(new StrategyStatusRequest());

    public async Task<ToggleStrategyReply> ToggleStrategyAsync(string strategy, bool enabled) =>
        await _client.ToggleStrategyAsync(new ToggleStrategyRequest { Strategy = strategy, Enabled = enabled });

    public void Dispose() => _channel.Dispose();
}
