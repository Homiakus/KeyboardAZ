package textinput

import "testing"

func BenchmarkResolveStrokeBase(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		action, err := ResolveStroke("ru", 0, 8)
		if err != nil || action.Text == "" {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveStrokeShiftRare(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		action, err := ResolveStroke("ru", ModifierShift|ModifierRare, 8)
		if err != nil || action.Text == "" {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveTap(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		action, err := ResolveTap("space")
		if err != nil || action.Key == "" {
			b.Fatal(err)
		}
	}
}
