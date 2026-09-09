package main

import (
	"log"
	"strconv"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	_ "projeto-rest/docs"
	"projeto-rest/internals/handler"
	"projeto-rest/internals/model"
	"projeto-rest/internals/repository"
)

// @title           Projeto REST API
// @version         1.0
// @description     Esta é uma API de exemplo utilizando Gin e Swagger.
// @host            localhost:8080
// @BasePath        /

// @Summary         Ping
// @Description     Responde com pong
// @Tags            System
// @Produce         json
// @Success         200  {object}  map[string]string
// @Router          /ping [get]
func pingHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "pong",
	})
}

// @Summary         Soma
// @Description     Soma dois números inteiros
// @Tags            Math
// @Produce         json
// @Success         200  {object}  map[string]int
// @Router          /soma/:a/:b [get]
func soma(c *gin.Context) {
	a, errA := strconv.Atoi(c.Param("a"))
	b, errB := strconv.Atoi(c.Param("b"))

	if errA != nil || errB != nil {
		c.JSON(400, gin.H{"error": "Parâmetros 'a' e 'b' devem ser numéricos"})
		return
	}

	c.JSON(200, gin.H{
		"soma": a + b,
	})
}

func main() {
	// Database setup
	db, err := gorm.Open(sqlite.Open("rest.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("Falha ao conectar no banco de dados sqlite:", err)
	}

	// Automigrate
	err = db.AutoMigrate(&model.User{})
	if err != nil {
		log.Fatal("Falha ao migrar banco de dados:", err)
	}

	// Injeção de dependências
	userRepo := repository.New(db)
	userHandler := handler.NewUserHandler(userRepo)

	router := gin.Default()

	// Configuração do CORS
	router.Use(cors.Default())

	// Redireciona /swagger para a interface do Swagger UI
	router.GET("/swagger", func(c *gin.Context) {
		c.Redirect(302, "/swagger/index.html")
	})

	// Rota para a documentação gerada pelo Swagger
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Rota do endpoint ping e soma
	router.GET("/ping", pingHandler)
	router.GET("/soma/:a/:b", soma)

	// Rotas de User
	router.GET("/users", userHandler.Findall)
	router.GET("/users/:id", userHandler.FindById)
	router.POST("/users", userHandler.Create)
	router.PUT("/users/:id", userHandler.Update)
	router.DELETE("/users/:id", userHandler.Delete)

	router.Run(":8080")
}
