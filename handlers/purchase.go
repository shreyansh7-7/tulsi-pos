package handlers

import (
	"context"
	"net/http"
	"time"

	"tulsi-pos/db"
	"tulsi-pos/utils"

	"github.com/gin-gonic/gin"
)

type PurchaseItem struct {
	ProductID     int     `json:"product_id"`
	Quantity      int     `json:"quantity"`
	PurchasePrice float64 `json:"purchase_price"`
	GSTPercent    float64 `json:"gst_percent"`
}

type PurchaseRequest struct {
	SupplierID  int            `json:"supplier_id"`
	InvoiceNo   string         `json:"invoice_number"`
	Notes       string         `json:"notes"`
	Items       []PurchaseItem `json:"items"`
	CreatedByID int            `json:"created_by"` // user id
}

func CreatePurchase(c *gin.Context) {
	var req PurchaseRequest
	if err := c.BindJSON(&req); err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(req.Items) == 0 {
		utils.SendErrorResponse(c, http.StatusBadRequest, "at least one item is required")
		return
	}

	ctx := context.Background()
	tx, err := db.DB.Begin(ctx)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, "failed to start tx")
		return
	}
	defer tx.Rollback(ctx)

	var totalAmountBeforeDiscount float64
	var totalGST float64
	var totalAmount float64
	var totalQty int

	// calculate totals
	for _, item := range req.Items {
		lineBase := float64(item.Quantity) * item.PurchasePrice
		gstAmt := lineBase * item.GSTPercent / 100.0
		lineTotal := lineBase + gstAmt

		totalAmountBeforeDiscount += lineBase
		totalGST += gstAmt
		totalAmount += lineTotal
		totalQty += item.Quantity
	}

	// insert into purchase_invoices
	var purchaseID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO purchase_invoices
		(invoice_number, supplier_id, total_amount_before_discount, discount_amount,
		 total_gst, total_invoice_amount, total_items, total_quantity, notes, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id
	`,
		req.InvoiceNo,
		req.SupplierID,
		totalAmountBeforeDiscount,
		0, // discount_amount for now
		totalGST,
		totalAmount,
		len(req.Items),
		totalQty,
		req.Notes,
		req.CreatedByID,
	).Scan(&purchaseID)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, "failed to insert purchase header: "+err.Error())
		return
	}

	// insert items + inventory_transactions
	for _, item := range req.Items {
		lineBase := float64(item.Quantity) * item.PurchasePrice
		gstAmt := lineBase * item.GSTPercent / 100.0
		lineTotal := lineBase + gstAmt

		_, err = tx.Exec(ctx, `
			INSERT INTO purchase_invoice_items
			(purchase_invoice_id, product_id, quantity, purchase_price,
			 gst_percent, gst_amount, line_total)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`,
			purchaseID,
			item.ProductID,
			item.Quantity,
			item.PurchasePrice,
			item.GSTPercent,
			gstAmt,
			lineTotal,
		)
		if err != nil {
			utils.SendErrorResponse(c, http.StatusInternalServerError, "failed to insert purchase item: "+err.Error())
			return
		}

		// inventory + stock (positive quantity)
		_, err = tx.Exec(ctx, `
			INSERT INTO inventory_transactions
			(product_id, quantity, ref_type, ref_id, created_by)
			VALUES ($1,$2,$3,$4,$5)
		`,
			item.ProductID,
			item.Quantity,
			"purchase",
			purchaseID,
			req.CreatedByID,
		)
		if err != nil {
			utils.SendErrorResponse(c, http.StatusInternalServerError, "failed to insert inventory tx: "+err.Error())
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, "failed to commit tx")
		return
	}

	utils.SendSuccessResponse(c, http.StatusCreated, gin.H{
		"purchase_id":    purchaseID,
		"total_amount":   totalAmount,
		"total_quantity": totalQty,
	}, "Purchase recorded")
}

func ListPurchases(c *gin.Context) {
	ctx := c.Request.Context()

	rows, err := db.DB.Query(ctx, `
		SELECT id, invoice_number, supplier_name, created_at
		FROM purchases
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	resp := []gin.H{}
	for rows.Next() {
		var r struct {
			ID            int64
			InvoiceNumber string
			SupplierName  string
			CreatedAt     time.Time
		}

		rows.Scan(&r.ID, &r.InvoiceNumber, &r.SupplierName, &r.CreatedAt)

		resp = append(resp, gin.H{
			"id":             r.ID,
			"invoice_number": r.InvoiceNumber,
			"supplier_name":  r.SupplierName,
			"created_at":     utils.FormatDateTime(r.CreatedAt),
		})
	}

	utils.SendSuccessResponse(c, http.StatusOK, resp, "Purchases fetched successfully")
}
