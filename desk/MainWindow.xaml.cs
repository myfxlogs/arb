using System.Windows;

namespace ArbDesk;

public partial class MainWindow : Window
{
    public MainWindow()
    {
        InitializeComponent();
        DataContext = new ViewModels.MainViewModel();
        Loaded += MainWindow_Loaded;
    }

    private async void MainWindow_Loaded(object sender, RoutedEventArgs e)
    {
        var vm = (ViewModels.MainViewModel)DataContext;
        await vm.InitializeAsync();
    }
}
