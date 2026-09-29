// Package realtime implementa o quadro colaborativo via WebSocket: presença
// dos doutores, cursores, arraste dos cards e notificações de mudanças feitas
// pela API REST. Cada card do quadro é uma passagem (uma vinda do paciente).
//
// Protocolo (JSON, campo "type"):
//
//	cliente -> servidor: cursor{x,y} | drag_start{passagem_id} | drag_move{passagem_id,x,y} | drag_end{passagem_id,x,y}
//	servidor -> cliente: init | join | leave | cursor | drag_start | drag_move | drag_denied |
//	                     passagem_updated | paciente_updated | paciente_deleted | sessao_encerrada
//
// Coordenadas são lógicas (model.QuadroLargura x model.QuadroAltura).
package realtime

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"projeto-rest/internals/middleware"
	"projeto-rest/internals/model"
	"projeto-rest/internals/pacientes"
	"projeto-rest/internals/repository"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	// Buffer generoso: a 60 msg/s por doutor, mensagens de cursor/arraste
	// podem ser descartadas se o cliente não acompanhar; as demais cabem.
	sendBuffer = 1024
)

var cores = []string{
	"#e6194b", "#3cb44b", "#4363d8", "#f58231", "#911eb4",
	"#42d4f4", "#f032e6", "#bfef45", "#fabed4", "#469990",
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Projeto local: aceita qualquer origem.
	CheckOrigin: func(r *http.Request) bool { return true },
}

type inbound struct {
	Type       string  `json:"type"`
	PassagemID uint    `json:"passagem_id"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
}

// Presenca é o que os outros doutores veem de uma conexão.
type Presenca struct {
	ConnID   uint64  `json:"conn_id"`
	DoutorID uint    `json:"doutor_id"`
	Nome     string  `json:"nome"`
	Cor      string  `json:"cor"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
}

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte

	connID   uint64
	doutorID uint
	nome     string
	cor      string

	mu sync.Mutex
	x  float64
	y  float64
}

func (c *Client) presenca() Presenca {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Presenca{ConnID: c.connID, DoutorID: c.doutorID, Nome: c.nome, Cor: c.cor, X: c.x, Y: c.y}
}

type Hub struct {
	repo repository.PassagemRepository
	dir  pacientes.Diretorio // pacientes vêm do serviço SOAP

	mu       sync.RWMutex
	clients  map[*Client]struct{}
	dragging map[uint]*Client // passagem_id -> quem está arrastando
	nextConn atomic.Uint64
}

func NewHub(repo repository.PassagemRepository, dir pacientes.Diretorio) *Hub {
	return &Hub{
		repo:     repo,
		dir:      dir,
		clients:  make(map[*Client]struct{}),
		dragging: make(map[uint]*Client),
	}
}

// ---- envio ----

func (c *Client) enqueue(data []byte) {
	select {
	case c.send <- data:
	default:
		// cliente lento: descarta a mensagem em vez de travar o hub
	}
}

func (h *Hub) broadcastExcept(msg any, except *Client) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("ws: marshal:", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c != except {
			c.enqueue(data)
		}
	}
}

// Broadcast envia a mensagem para todas as conexões.
func (h *Hub) Broadcast(msg any) { h.broadcastExcept(msg, nil) }

// NotifyPassagem avisa o quadro que uma passagem mudou (etapa, posição, documentos).
func (h *Hub) NotifyPassagem(p model.Passagem) {
	h.Broadcast(gin.H{"type": "passagem_updated", "passagem": p})
}

// NotifyPaciente avisa que os dados de um paciente mudaram (nome, status).
func (h *Hub) NotifyPaciente(p model.Paciente) {
	h.Broadcast(gin.H{"type": "paciente_updated", "paciente": p})
}

// NotifyPacienteRemovido avisa que o paciente e as suas passagens foram removidos.
func (h *Hub) NotifyPacienteRemovido(id uint, passagens []uint) {
	for _, pid := range passagens {
		h.ReleaseLock(pid)
	}
	h.Broadcast(gin.H{"type": "paciente_deleted", "paciente_id": id, "passagens": passagens})
}

// ReleaseLock libera o card (usado quando um documento é salvo pela API).
func (h *Hub) ReleaseLock(passagemID uint) {
	h.mu.Lock()
	delete(h.dragging, passagemID)
	h.mu.Unlock()
}

// DesconectarDoutor avisa as conexões do doutor que a sessão acabou (ele
// finalizou o plantão) e as fecha logo depois.
func (h *Hub) DesconectarDoutor(doutorID uint) {
	data, _ := json.Marshal(gin.H{"type": "sessao_encerrada"})
	h.mu.RLock()
	var conns []*Client
	for c := range h.clients {
		if c.doutorID == doutorID {
			conns = append(conns, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range conns {
		c.enqueue(data)
		conn := c.conn
		time.AfterFunc(500*time.Millisecond, func() { conn.Close() })
	}
}

// ---- ciclo de vida da conexão ----

// ServeWS faz o upgrade HTTP -> WebSocket. Deve ficar atrás do middleware JWT.
func (h *Hub) ServeWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("ws: upgrade:", err)
		return
	}

	doutorID := middleware.CurrentUserID(c)
	nome, _ := c.Get(middleware.ContextNome)
	nomeStr, _ := nome.(string)

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, sendBuffer),
		connID:   h.nextConn.Add(1),
		doutorID: doutorID,
		nome:     nomeStr,
		cor:      cores[int(doutorID)%len(cores)],
	}

	h.register(client)
	go client.writePump()
	client.readPump()
}

func (h *Hub) register(c *Client) {
	passagens, err := h.repo.FindQuadro(model.Hoje())
	if err != nil {
		log.Println("ws: quadro:", err)
	}
	h.dir.Anexar(passagens)

	h.mu.Lock()
	h.clients[c] = struct{}{}
	doutores := make([]Presenca, 0, len(h.clients))
	for other := range h.clients {
		if other != c {
			doutores = append(doutores, other.presenca())
		}
	}
	dragging := make(map[uint]uint64, len(h.dragging))
	for pid, owner := range h.dragging {
		dragging[pid] = owner.connID
	}
	h.mu.Unlock()

	init, _ := json.Marshal(gin.H{
		"type":      "init",
		"you":       c.presenca(),
		"doutores":  doutores,
		"passagens": passagens,
		"dragging":  dragging,
		"quadro": gin.H{
			"largura": model.QuadroLargura, "altura": model.QuadroAltura,
			"coluna": model.ColunaLargura, "card_largura": model.CardLargura, "card_altura": model.CardAltura,
			"etapas": model.Etapas,
		},
	})
	c.enqueue(init)

	h.broadcastExcept(gin.H{"type": "join", "doutor": c.presenca()}, c)
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	var liberados []uint
	for pid, owner := range h.dragging {
		if owner == c {
			delete(h.dragging, pid)
			liberados = append(liberados, pid)
		}
	}
	h.mu.Unlock()

	close(c.send)

	// Cards que estavam sendo arrastados voltam para a posição gravada.
	for _, pid := range liberados {
		if p, err := h.repo.FindById(pid); err == nil {
			pacientes.AnexarUma(h.dir, p)
			h.NotifyPassagem(*p)
		}
	}
	h.Broadcast(gin.H{"type": "leave", "conn_id": c.connID})
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister(c)
		c.conn.Close()
	}()
	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg inbound
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		c.hub.handle(c, msg)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case data, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ---- mensagens ----

func (h *Hub) handle(c *Client, msg inbound) {
	switch msg.Type {
	case "cursor":
		c.mu.Lock()
		c.x, c.y = msg.X, msg.Y
		c.mu.Unlock()
		h.broadcastExcept(gin.H{"type": "cursor", "conn_id": c.connID, "x": msg.X, "y": msg.Y}, c)

	case "drag_start":
		h.mu.Lock()
		owner, locked := h.dragging[msg.PassagemID]
		if locked && owner != c {
			h.mu.Unlock()
			data, _ := json.Marshal(gin.H{"type": "drag_denied", "passagem_id": msg.PassagemID, "conn_id": owner.connID})
			c.enqueue(data)
			return
		}
		h.dragging[msg.PassagemID] = c
		h.mu.Unlock()
		h.broadcastExcept(gin.H{"type": "drag_start", "conn_id": c.connID, "passagem_id": msg.PassagemID}, c)

	case "drag_move":
		if !h.owns(c, msg.PassagemID) {
			return
		}
		h.broadcastExcept(gin.H{"type": "drag_move", "conn_id": c.connID, "passagem_id": msg.PassagemID, "x": msg.X, "y": msg.Y}, c)

	case "drag_end":
		if !h.owns(c, msg.PassagemID) {
			return
		}
		h.ReleaseLock(msg.PassagemID)

		p, err := h.repo.FindById(msg.PassagemID)
		if err != nil {
			return
		}
		pacientes.AnexarUma(h.dir, p)
		// A posição só é gravada se o card foi solto na coluna da etapa atual.
		// Mudar de coluna exige o documento da etapa (via API REST); caso
		// contrário o card volta para a posição gravada.
		if p.Aberta() && model.ColunaDe(msg.X+model.CardLargura/2) == model.EtapaIndex(p.Etapa) {
			x, y := model.ClampPosicao(p.Etapa, msg.X, msg.Y)
			if err := h.repo.UpdatePosicao(p.ID, x, y); err == nil {
				p.PosX, p.PosY = x, y
			}
		}
		h.NotifyPassagem(*p)
	}
}

func (h *Hub) owns(c *Client, passagemID uint) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.dragging[passagemID] == c
}
