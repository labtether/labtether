package shared

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkEventBroadcast(b *testing.B) {
	for _, count := range []int{0, 1, 32, 128} {
		for _, size := range []int{64, 4096} {
			b.Run(fmt.Sprintf("clients=%d/bytes=%d", count, size), func(b *testing.B) {
				eb := NewEventBroadcaster()
				clients := make([]*BrowserClient, count)
				for i := range clients {
					clients[i] = NewBrowserClientForTesting(nil, make(chan []byte, 1))
					eb.InjectClientForTesting(clients[i])
				}
				data := map[string]any{"message": strings.Repeat("x", size)}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					eb.Broadcast("benchmark", data)
					for _, client := range clients {
						<-client.Outgoing
					}
				}
			})
		}
	}
}
