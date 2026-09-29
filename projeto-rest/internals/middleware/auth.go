package middleware

import (
	"net/http"
	"strings"
	"time"

	"projeto-rest/internals/auth"

	"github.com/gin-gonic/gin"
)

const (
	ContextUserID    = "userID"
	ContextNome      = "userNome"
	ContextEmitidoEm = "tokenEmitidoEm"
)

// SessaoEncerrada devolve quando o doutor finalizou o último plantão (nil se
// nunca finalizou). Tokens emitidos antes disso são recusados.
type SessaoEncerrada func(userID uint) *time.Time

// JWT exige o header "Authorization: Bearer <token>" e injeta o usuário no contexto.
// Como o navegador não consegue mandar headers ao abrir um WebSocket, o token
// também é aceito na query string (?token=...).
func JWT(svc *auth.Service, encerrada SessaoEncerrada) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			header = c.Query("token")
		}
		if header == "" {
			c.Header("WWW-Authenticate", `Bearer realm="projeto-rest"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token ausente"})
			return
		}

		// Aceita "Bearer <token>" (padrão) e também o token puro, que é o que o
		// Swagger UI envia quando se cola só o JWT no botão Authorize.
		token := strings.TrimSpace(header)
		if parts := strings.SplitN(token, " ", 2); len(parts) == 2 {
			if !strings.EqualFold(parts[0], "Bearer") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "formato esperado: Bearer <token>"})
				return
			}
			token = strings.TrimSpace(parts[1])
		}

		claims, err := svc.Parse(token)
		if err != nil {
			c.Header("WWW-Authenticate", `Bearer realm="projeto-rest", error="invalid_token"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token inválido ou expirado"})
			return
		}

		emitido := time.Time{}
		if claims.IssuedAt != nil {
			emitido = claims.IssuedAt.Time
		}
		// O iat do JWT tem precisão de segundos: compara no mesmo segundo.
		if fim := encerrada(claims.UserID); fim != nil && emitido.Before(fim.Truncate(time.Second)) {
			c.Header("WWW-Authenticate", `Bearer realm="projeto-rest", error="invalid_token"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "sessão encerrada ao finalizar o plantão"})
			return
		}

		c.Set(ContextUserID, claims.UserID)
		c.Set(ContextNome, claims.Nome)
		c.Set(ContextEmitidoEm, emitido)
		c.Next()
	}
}

// CurrentUserID lê o id do usuário autenticado colocado pelo middleware.
func CurrentUserID(c *gin.Context) uint {
	id, _ := c.Get(ContextUserID)
	uid, _ := id.(uint)
	return uid
}

// EmitidoEm é quando o token da requisição foi emitido (o login do doutor).
func EmitidoEm(c *gin.Context) time.Time {
	v, _ := c.Get(ContextEmitidoEm)
	t, _ := v.(time.Time)
	return t
}
