package dto

// LoginDTO é o corpo JSON aceito pelo endpoint de token.
// O endpoint também aceita o formato form-urlencoded do OAuth2
// (grant_type=password&username=...&password=...).
type LoginDTO struct {
	Nome  string `json:"nome" form:"username" binding:"required"`
	Senha string `json:"senha" form:"password" binding:"required"`
}

// TokenResponse segue o formato de resposta do OAuth2 (RFC 6749, seção 5.1).
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type" example:"Bearer"`
	ExpiresIn   int64  `json:"expires_in" example:"3600"`
}

// OAuthError segue o formato de erro do OAuth2 (RFC 6749, seção 5.2).
type OAuthError struct {
	Error            string `json:"error" example:"invalid_grant"`
	ErrorDescription string `json:"error_description,omitempty"`
}
