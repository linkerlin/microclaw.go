// Package web implements the HTTP/WebSocket web channel for microclaw.
package web

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/linkerlin/microclaw.go/internal/agent"
	"github.com/linkerlin/microclaw.go/internal/config"
	"github.com/linkerlin/microclaw.go/internal/db"
	"github.com/linkerlin/microclaw.go/internal/memory"
)

const channelName = "web"

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Channel is the web server channel.
type Channel struct {
	cfg    *config.Config
	db     *db.DB
	mem    *memory.Manager
	engine *agent.Engine
	server *http.Server
	mu     sync.Mutex
}

// New creates a new web Channel.
func New(cfg *config.Config, database *db.DB, mem *memory.Manager, eng *agent.Engine) *Channel {
	c := &Channel{
		cfg:    cfg,
		db:     database,
		mem:    mem,
		engine: eng,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", c.serveIndex)
	mux.HandleFunc("/ws", c.serveWebSocket)
	mux.HandleFunc("/api/messages", c.serveMessages)
	mux.HandleFunc("/api/clear", c.serveClear)

	c.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.WebHost, cfg.WebPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	return c
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (c *Channel) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", c.server.Addr)
	if err != nil {
		return fmt.Errorf("binding to %s: %w", c.server.Addr, err)
	}
	log.Printf("Web channel listening on http://%s", c.server.Addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.server.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return c.server.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

// wsMessage is a WebSocket message frame.
type wsMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	ChatID  int64  `json:"chat_id,omitempty"`
}

func (c *Channel) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Use remote addr as unique chat ID (simple hash).
	chatID := int64(hashString(r.RemoteAddr))

	for {
		var msg wsMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		if msg.Type != "message" || msg.Content == "" {
			continue
		}

		// Save user message.
		_ = c.db.SaveMessage(&db.Message{
			ChatID:      chatID,
			ChatChannel: channelName,
			SenderName:  "user",
			Content:     msg.Content,
			IsFromBot:   false,
			Timestamp:   time.Now(),
		})

		// Send typing indicator.
		_ = conn.WriteJSON(wsMessage{Type: "typing"})

		// Process message.
		resp, err := c.engine.Process(r.Context(), agent.ChatRequest{
			ChatID:   chatID,
			Channel:  channelName,
			UserName: "web-user",
			Message:  msg.Content,
		})

		if err != nil {
			log.Printf("Agent error: %v", err)
			_ = conn.WriteJSON(wsMessage{Type: "error", Content: "Error processing your message."})
			continue
		}

		text := resp.Text
		if text == "" {
			text = "I couldn't generate a response."
		}

		// Save bot response.
		_ = c.db.SaveMessage(&db.Message{
			ChatID:      chatID,
			ChatChannel: channelName,
			SenderName:  "microclaw",
			Content:     text,
			IsFromBot:   true,
			Timestamp:   time.Now(),
		})

		_ = conn.WriteJSON(wsMessage{Type: "message", Content: text})
	}
}

func (c *Channel) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func (c *Channel) serveMessages(w http.ResponseWriter, r *http.Request) {
	chatID := int64(hashString(r.RemoteAddr))
	msgs, err := c.db.GetMessages(chatID, channelName, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msgs)
}

func (c *Channel) serveClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	chatID := int64(hashString(r.RemoteAddr))
	_ = c.engine.ClearSession(r.Context(), chatID, channelName)
	_ = c.db.DeleteMessages(chatID, channelName)
	w.WriteHeader(http.StatusOK)
}

// checkPassword validates the password with constant-time comparison.
func checkPassword(hash, input string) bool {
	return subtle.ConstantTimeCompare([]byte(hash), []byte(input)) == 1
}

func hashString(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// indexHTML is the embedded web UI.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>MicroClaw</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: system-ui, sans-serif; background: #1a1a2e; color: #e0e0e0; height: 100vh; display: flex; flex-direction: column; }
#header { background: #16213e; padding: 12px 20px; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #0f3460; }
#header h1 { font-size: 1.2rem; color: #e94560; }
#clear-btn { background: #e94560; color: white; border: none; padding: 6px 12px; border-radius: 6px; cursor: pointer; }
#messages { flex: 1; overflow-y: auto; padding: 20px; display: flex; flex-direction: column; gap: 12px; }
.msg { max-width: 80%; padding: 10px 14px; border-radius: 12px; line-height: 1.5; white-space: pre-wrap; word-break: break-word; }
.msg.user { background: #0f3460; align-self: flex-end; }
.msg.bot { background: #16213e; border: 1px solid #0f3460; align-self: flex-start; }
.msg.typing { background: #16213e; border: 1px solid #0f3460; align-self: flex-start; color: #888; font-style: italic; }
#input-area { padding: 16px 20px; background: #16213e; border-top: 1px solid #0f3460; display: flex; gap: 10px; }
#input { flex: 1; background: #1a1a2e; border: 1px solid #0f3460; color: #e0e0e0; padding: 10px 14px; border-radius: 8px; font-size: 1rem; resize: none; height: 44px; }
#send-btn { background: #e94560; color: white; border: none; padding: 10px 20px; border-radius: 8px; cursor: pointer; font-size: 1rem; }
#send-btn:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
</head>
<body>
<div id="header">
  <h1>🐾 MicroClaw</h1>
  <button id="clear-btn" onclick="clearChat()">Clear</button>
</div>
<div id="messages"></div>
<div id="input-area">
  <textarea id="input" placeholder="Type a message..." onkeydown="handleKey(event)"></textarea>
  <button id="send-btn" onclick="sendMessage()">Send</button>
</div>
<script>
const messages = document.getElementById('messages');
const input = document.getElementById('input');
const sendBtn = document.getElementById('send-btn');
let ws, typingEl;

function connect() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  ws = new WebSocket(proto + '//' + location.host + '/ws');
  ws.onmessage = onMessage;
  ws.onclose = () => setTimeout(connect, 2000);
}

function onMessage(e) {
  const msg = JSON.parse(e.data);
  if (msg.type === 'typing') {
    if (!typingEl) {
      typingEl = addMsg('Thinking...', 'typing');
    }
  } else if (msg.type === 'message') {
    if (typingEl) { typingEl.remove(); typingEl = null; }
    addMsg(msg.content, 'bot');
    sendBtn.disabled = false;
  } else if (msg.type === 'error') {
    if (typingEl) { typingEl.remove(); typingEl = null; }
    addMsg('Error: ' + msg.content, 'bot');
    sendBtn.disabled = false;
  }
}

function addMsg(text, cls) {
  const el = document.createElement('div');
  el.className = 'msg ' + cls;
  el.textContent = text;
  messages.appendChild(el);
  messages.scrollTop = messages.scrollHeight;
  return el;
}

function sendMessage() {
  const text = input.value.trim();
  if (!text || !ws || ws.readyState !== WebSocket.OPEN) return;
  addMsg(text, 'user');
  ws.send(JSON.stringify({ type: 'message', content: text }));
  input.value = '';
  sendBtn.disabled = true;
}

function handleKey(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendMessage();
  }
}

function clearChat() {
  fetch('/api/clear', { method: 'POST' }).then(() => {
    messages.innerHTML = '';
    addMsg('Conversation cleared.', 'bot');
  });
}

connect();
</script>
</body>
</html>`


