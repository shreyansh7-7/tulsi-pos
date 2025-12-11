package main

import (
	"log"
	"strconv"
	awsclient "tulsi-pos/aws"
	"tulsi-pos/db"
	"tulsi-pos/handlers"
	"tulsi-pos/middleware"
	"tulsi-pos/services"

	"github.com/gin-gonic/gin"
)

func main() {
	if err := db.InitDB(); err != nil {
		log.Fatal(err)
	}

	awsclient.InitAWS()

	r := gin.Default()

	r.POST("/products", handlers.CreateProduct)
	r.GET("/products", handlers.GetProducts)

	// r.POST("/purchases", handlers.CreatePurchase)
	// r.POST("/sales", handlers.CreateSale)
	r.POST("/purchases", middleware.AuthRequired(), handlers.CreatePurchase)
	// r.POST("/sales", middleware.AuthRequired(), handlers.CreateSale)

	r.POST("/sales-invoices", handlers.CreateInvoice)
	r.PUT("/sales-invoices/:id", handlers.UpdateInvoice)

	// middleware.RequireRole("admin")

	r.POST("/auth/login", handlers.Login)
	r.POST("/auth/register", handlers.Register)

	r.POST("/invoices/:id/generate-pdf", func(c *gin.Context) {
		idStr := c.Param("id")
		id, _ := strconv.ParseInt(idStr, 10, 64)

		key, err := services.GenerateAndUploadInvoicePDF(c.Request.Context(), id)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"pdf_key": key})
	})

	r.GET("/products/:id", handlers.GetProductByID)

	r.GET("/sales/invoices/:id", handlers.GetInvoiceByID)
	r.GET("/sales/invoices", handlers.ListInvoices)

	r.GET("/purchases", handlers.ListPurchases)


	r.Run(":8080")
}
