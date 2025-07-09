// client/ws.go
package client

import (
    "net/http"

    "github.com/Coop25/CC-Radio/manager"
    "github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func RegisterWS(b *manager.Broadcaster) {
    http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
        conn, err := upgrader.Upgrade(w, r, nil)
        if err != nil {
            return
        }
        b.Register(conn)
        defer b.Unregister(conn)
        // block reads until disconnect
        for {
            if _, _, err := conn.ReadMessage(); err != nil {
                break
            }
        }
    })
}
