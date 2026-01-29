package handlers

import (
	"context"
	"net/http"
	"strconv"

	"tulsi-pos/db"
	"tulsi-pos/utils"

	"github.com/gin-gonic/gin"
)

type Product struct {
	Name          string  `json:"name"`
	SKU           string  `json:"sku"`
	Barcode       string  `json:"barcode"`
	HSNCode       string  `json:"hsn_code"`
	Gender        string  `json:"gender"`
	Category      string  `json:"category"`
	PurchasePrice float64 `json:"purchase_price"`
	SalesPrice    float64 `json:"sales_price"`
	GSTPercent    float64 `json:"gst_percent"`
}

// POST /products
func CreateProduct(c *gin.Context) {
	var p Product

	if err := c.BindJSON(&p); err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "Invalid JSON")
		return
	}

	query := `
		INSERT INTO products
		(name, sku, barcode, hsn_code, gender, category, purchase_price, sales_price, gst_percent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id
	`

	var id int
	err := db.DB.QueryRow(
		context.Background(),
		query,
		p.Name, p.SKU, p.Barcode, p.HSNCode, p.Gender, p.Category,
		p.PurchasePrice, p.SalesPrice, p.GSTPercent,
	).Scan(&id)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	utils.SendSuccessResponse(c, http.StatusCreated, gin.H{"id": id}, "Product created")
}

// GET /products
func GetProducts(c *gin.Context) {
	rows, err := db.DB.Query(context.Background(), `
		SELECT id, name, sku, barcode, category, gender, sales_price
		FROM products
		WHERE deleted_at IS NULL
	`)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	products := []map[string]interface{}{}

	for rows.Next() {
		var id int
		var name, sku, barcode, category, gender string
		var salesPrice float64

		rows.Scan(&id, &name, &sku, &barcode, &category, &gender, &salesPrice)

		products = append(products, gin.H{
			"id":          id,
			"name":        name,
			"sku":         sku,
			"barcode":     barcode,
			"category":    category,
			"gender":      gender,
			"sales_price": salesPrice,
		})
	}

	utils.SendSuccessResponse(c, http.StatusOK, products, "Products fetched successfully")
}

func GetProductByID(c *gin.Context) {
	idStr := c.Param("id")
	productID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || productID <= 0 {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid product id")
		return
	}

	var p struct {
		ID            int64   `json:"id"`
		Name          string  `json:"name"`
		SKU           string  `json:"sku"`
		Barcode       string  `json:"barcode"`
		HSNCode       string  `json:"hsn_code"`
		Gender        string  `json:"gender"`
		Category      string  `json:"category"`
		PurchasePrice float64 `json:"purchase_price"`
		SalesPrice    float64 `json:"sales_price"`
		GSTPercent    float64 `json:"gst_percent"`
	}

	err = db.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, sku, barcode, hsn_code, gender, category,
		       purchase_price, sales_price, gst_percent
		FROM products
		WHERE id = $1 AND deleted_at IS NULL
	`, productID).Scan(
		&p.ID, &p.Name, &p.SKU, &p.Barcode, &p.HSNCode,
		&p.Gender, &p.Category, &p.PurchasePrice, &p.SalesPrice, &p.GSTPercent,
	)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusNotFound, "product not found")
		return
	}

	utils.SendSuccessResponse(c, http.StatusOK, p, "Product details fetched successfully")
}
