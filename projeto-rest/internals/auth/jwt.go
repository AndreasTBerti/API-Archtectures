package auth

import (
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "projeto-rest"

// Claims são os dados carregados dentro do JWT.
type Claims struct {
	UserID uint   `json:"uid"`
	Nome   string `json:"nome"`
	jwt.RegisteredClaims
}

// Service gera e valida tokens JWT (HS256).
type Service struct {
	secret []byte
	ttl    time.Duration
}

// NewFromEnv lê JWT_SECRET e JWT_TTL_MINUTES do ambiente.
func NewFromEnv() *Service {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-troque-em-producao"
		log.Println("AVISO: JWT_SECRET não definido, usando segredo de desenvolvimento")
	}

	ttl := 60 * time.Minute
	if v := os.Getenv("JWT_TTL_MINUTES"); v != "" {
		if m, err := strconv.Atoi(v); err == nil && m > 0 {
			ttl = time.Duration(m) * time.Minute
		}
	}

	return &Service{secret: []byte(secret), ttl: ttl}
}

// TTL retorna a validade dos tokens emitidos.
func (s *Service) TTL() time.Duration { return s.ttl }

// Generate emite um token assinado para o usuário.
func (s *Service) Generate(userID uint, nome string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Nome:   nome,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// Parse valida a assinatura e a expiração e devolve as claims.
func (s *Service) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de assinatura inválido")
		}
		return s.secret, nil
	}, jwt.WithIssuer(issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("token inválido")
	}
	return claims, nil
}
