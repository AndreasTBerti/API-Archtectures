package main

import (
	"log"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	_ "projeto-rest/docs"
	"projeto-rest/internals/auth"
	"projeto-rest/internals/handler"
	"projeto-rest/internals/middleware"
	"projeto-rest/internals/model"
	"projeto-rest/internals/pacientes"
	"projeto-rest/internals/realtime"
	"projeto-rest/internals/repository"
	"projeto-rest/internals/resumo"
	"projeto-rest/internals/soap"
)

// @title           Projeto REST API
// @version         2.0
// @description     Sistema de controle de pacientes. Os doutores fazem login (OAuth2 password + JWT). Os dados do paciente (cadastro, consulta, alteração, remoção) ficam no serviço SOAP (projeto-soap); este REST guarda os documentos gerados em cada vinda e as finalizações de plantão. Cada vinda do paciente ao hospital é uma passagem (uma por dia) com os seus documentos: a admissão abre a passagem em aguardando_triagem, a triagem leva para aguardando_consulta e a consulta dá alta. O resumo do paciente usa só as passagens anteriores. Quadro em tempo real em GET / (WebSocket em /ws?token=...).
// @host            localhost:8080
// @BasePath        /

// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Cole o access_token obtido em POST /auth/token (com ou sem o prefixo "Bearer ")

// @Summary         Ping
// @Description     Responde com pong
// @Tags            System
// @Produce         json
// @Success         200  {object}  map[string]string
// @Router          /health [get]
func pingHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

// abrirBanco abre o SQLite no caminho informado e aplica as migrações.
func abrirBanco(caminho string) (*gorm.DB, error) {
	// "record not found" é resultado normal de busca (ex.: paciente sem
	// passagem aberta), não erro: não aparece mais em vermelho no console.
	gormLog := logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  true,
	})
	db, err := gorm.Open(sqlite.Open(caminho), &gorm.Config{Logger: gormLog})
	if err != nil {
		return nil, err
	}
	if err := migrar(db); err != nil {
		return nil, err
	}
	return db, nil
}

func main() {
	caminho := os.Getenv("REST_DB")
	if caminho == "" {
		caminho = "rest.db"
	}
	db, err := abrirBanco(caminho)
	if err != nil {
		log.Fatal("Falha ao abrir o banco de dados sqlite:", err)
	}

	router := montarRouter(db, auth.NewFromEnv(), soap.NewFromEnv())

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	router.Run(":" + port)
}

// montarRouter faz a injeção de dependências e registra as rotas. Fica fora
// do main para os testes de integração subirem a mesma aplicação.
func montarRouter(db *gorm.DB, jwtService *auth.Service, soapClient *soap.Client) *gin.Engine {
	userRepo := repository.New(db)
	passagemRepo := repository.NewPassagemRepository(db)
	finalizacaoRepo := repository.NewFinalizacaoRepository(db)
	diretorio := pacientes.NewSoap(soapClient)

	hub := realtime.NewHub(passagemRepo, diretorio)

	authHandler := handler.NewAuthHandler(userRepo, jwtService)
	userHandler := handler.NewUserHandler(userRepo)
	pacienteHandler := handler.NewPacienteHandler(diretorio, passagemRepo, hub)
	passagemHandler := handler.NewPassagemHandler(passagemRepo, diretorio, finalizacaoRepo, hub, resumo.Regras{})
	finalizacaoHandler := handler.NewFinalizacaoHandler(finalizacaoRepo, userRepo, passagemRepo, diretorio, hub)

	// Tokens emitidos antes da última finalização de plantão do doutor não valem mais.
	sessaoEncerrada := func(id uint) *time.Time {
		var u model.User
		if err := db.Select("sessao_encerrada_em").First(&u, id).Error; err != nil {
			return nil
		}
		return u.SessaoEncerradaEm
	}

	router := gin.Default()

	// Configuração do CORS: o quadro pode ser aberto de outra origem (Live Server,
	// file://), então libera o header Authorization além do padrão.
	corsCfg := cors.DefaultConfig()
	corsCfg.AllowAllOrigins = true
	corsCfg.AllowHeaders = append(corsCfg.AllowHeaders, "Authorization")
	router.Use(cors.New(corsCfg))

	// Redireciona /swagger para a interface do Swagger UI
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(302, "/swagger/index.html")
	})

	// Rota para a documentação gerada pelo Swagger
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Rota do endpoint ping
	router.GET("/ping", pingHandler)
	router.GET("/health", pingHandler)

	// Frontend do quadro em tempo real (arquivo único)
	router.StaticFile("/", "./web/index.html")

	// Rotas públicas de autenticação
	router.POST("/auth/register", authHandler.Register)
	router.POST("/auth/token", authHandler.Token)

	// Tudo abaixo exige "Authorization: Bearer <jwt>"
	protected := router.Group("/", middleware.JWT(jwtService, sessaoEncerrada))

	protected.GET("/auth/me", authHandler.Me)

	// Quadro em tempo real (o token vai na query string: /ws?token=<jwt>)
	protected.GET("/ws", hub.ServeWS)
	protected.GET("/quadro", passagemHandler.Quadro)

	// Rotas de User (doutores)
	protected.GET("/users", userHandler.Findall)
	protected.GET("/users/:id", userHandler.FindById)
	protected.POST("/users", userHandler.Create)
	protected.PUT("/users/:id", userHandler.Update)
	protected.DELETE("/users/:id", userHandler.Delete)

	// Rotas de Paciente: repassadas ao serviço SOAP, que é o dono desses dados
	protected.GET("/pacientes", pacienteHandler.Findall)
	protected.POST("/pacientes", pacienteHandler.Create)
	protected.GET("/pacientes/:id", pacienteHandler.FindById)
	protected.PUT("/pacientes/:id", pacienteHandler.Update)
	protected.DELETE("/pacientes/:id", pacienteHandler.Delete)

	// Passagens: cada vinda do paciente, com os documentos de cada etapa
	protected.GET("/pacientes/:id/passagens", passagemHandler.ListarDoPaciente)
	protected.POST("/pacientes/:id/passagens", passagemHandler.RegistrarChegada)
	protected.GET("/pacientes/:id/resumo", passagemHandler.Resumo)
	protected.GET("/passagens/:id", passagemHandler.FindById)
	protected.POST("/passagens/:id/admissao", passagemHandler.Admissao)
	protected.POST("/passagens/:id/triagem", passagemHandler.Triagem)
	protected.POST("/passagens/:id/consulta", passagemHandler.Consulta)

	// Finalização do plantão (resumo, comentários e logout)
	protected.GET("/finalizacao", finalizacaoHandler.Previa)
	protected.POST("/finalizacao", finalizacaoHandler.Finalizar)
	protected.GET("/finalizacoes", finalizacaoHandler.Listar)

	return router
}
