// client/ws.go
package client

import (
	"log"
	"net/http"

	"github.com/Coop25/CC-Radio/manager"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow all origins (adjust if you need stricter CORS)
	},
}

func RegisterWS(b *manager.Broadcaster) {
	http.HandleFunc("/ws", wsHandler(b))
}

func wsHandler(b *manager.Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1) Perform the Upgrade
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("WebSocket upgrade failed:", err)
			return
		}

		// 2) Register this connection with the broadcaster
		b.Register(conn)
		defer func() {
			b.Unregister(conn)
			conn.Close()
		}()

		// 3) Block until client disconnects
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				// client closed or network error
				break
			}
		}
	}
}
