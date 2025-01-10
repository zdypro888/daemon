package main

import (
	"net/http"

	"github.com/zdypro888/daemon"
	_ "github.com/zdypro888/daemon/examples/server/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

//	@title			Sample Gin Swagger Server
//	@version		1.0
//	@description	This is a sample server for Gin Swagger
//	@termsOfService	https://nb-intelligence.com

//	@contact.name	Support
//	@contact.url	https://nb-intelligence.com
//	@contact.email	info@nb-intelligence.com

//	@license.name	Privacy
//	@license.url	https://nb-intelligence.com

//	@host		localhost
//	@BasePath	/api/v1

// PingResponse ping response
type PingResponse struct {
	Message string `json:"message"`
}

// PingHandler ping/pong example
//
//	@Summary		Ping example
//	@Description	Do ping
//	@Tags			example
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	PingResponse
//	@Router			/ping [get]
func PingHandler(c *gin.Context) {
	c.JSON(http.StatusOK, &PingResponse{Message: "pong"})
}

func main() {
	engine := daemon.NewEngine()
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	api := engine.Group("/api/v1")
	{
		api.GET("/ping", PingHandler)
	}
	engine.Start("")
	engine.Graceful()
}
