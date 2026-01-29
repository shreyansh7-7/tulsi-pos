package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tulsi-pos/db"
	"tulsi-pos/services"
	"tulsi-pos/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// ---------- Request DTOs ----------

type InvoiceItemInput struct {
	ProductID     int64   `json:"product_id" binding:"required"`
	Quantity      int     `json:"quantity" binding:"required,min=1"`
	MRP           float64 `json:"mrp"`
	SalesRate     float64 `json:"sales_rate" binding:"required"`
	DiscountType  string  `json:"discount_type"`  // "INR" or "%"
	DiscountValue float64 `json:"discount_value"` // flat or percent
	GSTPercent    float64 `json:"gst_percent"`
}

type InvoiceInput struct {
	CustomerName   string             `json:"customer_name"`
	CustomerMobile string             `json:"customer_mobile"`
	PaymentMode    string             `json:"payment_mode"` // cash/card/upi
	IsConfirmed    bool               `json:"is_confirmed"` // true => INVOICED
	Items          []InvoiceItemInput `json:"items" binding:"required,min=1"`
}

type invoiceMetaDTO struct {
	CustomerName   string      `json:"customer_name"`
	CustomerMobile string      `json:"customer_mobile"`
	PaymentMode    string      `json:"payment_mode"`
	IsConfirmed    interface{} `json:"is_confirmed"` // can be bool or 0/1 number or "true"/"false"
}
type invoiceRequestDTO struct {
	Invoice invoiceMetaDTO     `json:"invoice"`
	Items   []InvoiceItemInput `json:"items" binding:"required,min=1"`
}

// ---------- Public Handlers ----------

// POST /sales/invoices
func CreateInvoice(c *gin.Context) {
	var req invoiceRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload", "details": err.Error()})
		return
	}

	// convert wrapper -> internal InvoiceInput
	in, err := convertRequestToInvoiceInput(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload", "details": err.Error()})
		return
	}

	ctx := c.Request.Context()

	invoiceID, finalStatus, err := upsertInvoice(ctx, nil, in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Generate PDF only if final status is INVOICED
	if finalStatus == "INVOICED" {
		if key, err := services.GenerateAndUploadInvoicePDF(ctx, invoiceID); err != nil {
			log.Printf("failed to generate/upload invoice pdf for %d: %v", invoiceID, err)
		} else {
			log.Printf("invoice %d pdf uploaded to %s", invoiceID, key)
		}
	}

	utils.SendSuccessResponse(c, http.StatusCreated, gin.H{
		"invoice_id":   invoiceID,
		"final_status": finalStatus,
	}, "invoice created")
}

// PUT /sales/invoices/:id
func UpdateInvoice(c *gin.Context) {
	idStr := c.Param("id")
	invoiceID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || invoiceID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice id"})
		return
	}

	var req invoiceRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload", "details": err.Error()})
		return
	}

	in, err := convertRequestToInvoiceInput(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload", "details": err.Error()})
		return
	}

	ctx := c.Request.Context()

	idPtr := &invoiceID
	updatedID, finalStatus, err := upsertInvoice(ctx, idPtr, in)
	if err != nil {
		if err == ErrInvoiceLocked {
			utils.SendErrorResponse(c, http.StatusBadRequest, "invoice already invoiced, cannot update")
			return
		}
		if err == ErrInvoiceNotFound {
			utils.SendErrorResponse(c, http.StatusNotFound, "invoice not found")
			return
		}
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	if finalStatus == "INVOICED" {
		if key, err := services.GenerateAndUploadInvoicePDF(ctx, updatedID); err != nil {
			log.Printf("failed to generate/upload invoice pdf for %d: %v", updatedID, err)
		} else {
			log.Printf("invoice %d pdf uploaded to %s", updatedID, key)
		}
	}

	utils.SendSuccessResponse(c, http.StatusOK, gin.H{
		"invoice_id":   updatedID,
		"final_status": finalStatus,
	}, "invoice updated")
}

// ---------- Internal Logic ----------

var (
	ErrInvoiceLocked   = fmt.Errorf("invoice is already invoiced")
	ErrInvoiceNotFound = fmt.Errorf("invoice not found")
)

// convertRequestToInvoiceInput converts the incoming wrapper into the existing InvoiceInput
func convertRequestToInvoiceInput(r invoiceRequestDTO) (InvoiceInput, error) {
	in := InvoiceInput{
		CustomerName:   r.Invoice.CustomerName,
		CustomerMobile: r.Invoice.CustomerMobile,
		PaymentMode:    r.Invoice.PaymentMode,
		Items:          r.Items,
	}

	// parse IsConfirmed (supports bool, number, string)
	in.IsConfirmed = parseBoolish(r.Invoice.IsConfirmed)

	return in, nil
}

// parseBoolish accepts bool / numeric (0/1) / "true"/"false" and returns bool
func parseBoolish(v interface{}) bool {
	if v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case int:
		return t != 0
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		if s == "true" || s == "1" {
			return true
		}
		return false
	default:
		return false
	}
}

// shared function used by both create & update
// invoiceID == nil  → create
// invoiceID != nil  → update
func upsertInvoice(ctx context.Context, invoiceID *int64, in InvoiceInput) (int64, string, error) {
	tx, err := db.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	fmt.Println("in.IsConfirmed", in.IsConfirmed)
	fmt.Println("in.Items", in.Items)
	fmt.Println("in.CustomerName", in.CustomerName)
	fmt.Println("in.CustomerMobile", in.CustomerMobile)
	fmt.Println("in.PaymentMode", in.PaymentMode)
	fmt.Println("invoiceID", invoiceID)

	// If update, load existing status
	var existingStatus string
	var id int64
	if invoiceID != nil {
		err = tx.QueryRow(ctx, `
			SELECT id, status
			FROM sales_invoices
			WHERE id = $1 AND deleted_at IS NULL
		`, *invoiceID).Scan(&id, &existingStatus)

		if err == pgx.ErrNoRows {
			return 0, "", ErrInvoiceNotFound
		}
		if err != nil {
			return 0, "", fmt.Errorf("load invoice: %w", err)
		}
		if existingStatus == "INVOICED" {
			return 0, "", ErrInvoiceLocked
		}
	} else {
		id = 0
		existingStatus = "DRAFT"
	}

	// Compute totals
	totalAmountBeforeDiscount := 0.0
	totalDiscount := 0.0
	taxableAmount := 0.0
	totalGST := 0.0
	totalInvoiceAmount := 0.0
	totalItems := len(in.Items)
	totalQuantity := 0

	for _, it := range in.Items {
		gross := float64(it.Quantity) * it.SalesRate
		totalAmountBeforeDiscount += gross
		totalQuantity += it.Quantity

		discAmount := 0.0
		switch it.DiscountType {
		case "%", "PCT", "PERCENT":
			discAmount = gross * it.DiscountValue / 100.0
		case "INR", "", "FLAT":
			discAmount = it.DiscountValue
		default:
			discAmount = it.DiscountValue
		}
		if discAmount < 0 {
			discAmount = 0
		}
		totalDiscount += discAmount

		taxable := gross - discAmount
		if taxable < 0 {
			taxable = 0
		}
		taxableAmount += taxable

		gstAmount := taxable * it.GSTPercent / 100.0
		totalGST += gstAmount

		lineTotal := taxable + gstAmount
		totalInvoiceAmount += lineTotal
	}

	roundedTotal := math.Round(totalInvoiceAmount)
	roundOff := roundedTotal - totalInvoiceAmount
	totalInvoiceAmount = roundedTotal

	finalStatus := "DRAFT"
	if in.IsConfirmed {
		finalStatus = "INVOICED"
	}

	now := time.Now()

	// Insert or update invoice header
	if invoiceID == nil {
		invoiceNumber := fmt.Sprintf("INV%s%04d", now.Format("20060102"), now.UnixNano()%10000)

		err = tx.QueryRow(ctx, `
			INSERT INTO sales_invoices (
				invoice_number,
				customer_name,
				customer_mobile,
				status,
				total_amount_before_discount,
				discount_type,
				discount_value,
				total_discount,
				taxable_amount,
				total_gst,
				round_off,
				total_invoice_amount,
				total_items,
				total_quantity,
				payment_mode
			) VALUES (
				$1,$2,$3,$4,
				$5,$6,$7,$8,
				$9,$10,$11,$12,
				$13,$14,$15
			)
			RETURNING id
		`,
			invoiceNumber,
			in.CustomerName,
			in.CustomerMobile,
			finalStatus,
			totalAmountBeforeDiscount,
			normalizeDiscountType(in.Items),
			in.Items[0].DiscountValue, // simple: first item discount_value for header
			totalDiscount,
			taxableAmount,
			totalGST,
			roundOff,
			totalInvoiceAmount,
			totalItems,
			totalQuantity,
			in.PaymentMode,
		).Scan(&id)
		if err != nil {
			return 0, "", fmt.Errorf("insert invoice: %w", err)
		}
	} else {
		// update header
		_, err = tx.Exec(ctx, `
			UPDATE sales_invoices
			SET customer_name = $1,
			    customer_mobile = $2,
			    status = $3,
			    total_amount_before_discount = $4,
			    discount_type = $5,
			    discount_value = $6,
			    total_discount = $7,
			    taxable_amount = $8,
			    total_gst = $9,
			    round_off = $10,
			    total_invoice_amount = $11,
			    total_items = $12,
			    total_quantity = $13,
			    payment_mode = $14,
			    updated_at = $15
			WHERE id = $16 AND deleted_at IS NULL
		`,
			in.CustomerName,
			in.CustomerMobile,
			finalStatus,
			totalAmountBeforeDiscount,
			normalizeDiscountType(in.Items),
			in.Items[0].DiscountValue,
			totalDiscount,
			taxableAmount,
			totalGST,
			roundOff,
			totalInvoiceAmount,
			totalItems,
			totalQuantity,
			in.PaymentMode,
			now,
			id,
		)
		if err != nil {
			return 0, "", fmt.Errorf("update invoice: %w", err)
		}

		// soft delete old items
		_, err = tx.Exec(ctx, `
			UPDATE sales_invoice_items
			SET deleted_at = $1
			WHERE sales_invoice_id = $2 AND deleted_at IS NULL
		`, now, id)
		if err != nil {
			return 0, "", fmt.Errorf("soft delete old items: %w", err)
		}
	}

	// Insert new items
	for _, it := range in.Items {
		gross := float64(it.Quantity) * it.SalesRate

		discAmount := 0.0
		switch it.DiscountType {
		case "%", "PCT", "PERCENT":
			discAmount = gross * it.DiscountValue / 100.0
		case "INR", "", "FLAT":
			discAmount = it.DiscountValue
		default:
			discAmount = it.DiscountValue
		}
		if discAmount < 0 {
			discAmount = 0
		}

		taxable := gross - discAmount
		if taxable < 0 {
			taxable = 0
		}
		gstAmount := taxable * it.GSTPercent / 100.0
		lineTotal := taxable + gstAmount

		_, err = tx.Exec(ctx, `
			INSERT INTO sales_invoice_items (
				sales_invoice_id,
				product_id,
				quantity,
				mrp,
				sales_rate,
				discount_type,
				discount_value,
				discount_amount,
				gst_percent,
				gst_amount,
				line_total
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
			)
		`,
			id,
			it.ProductID,
			it.Quantity,
			it.MRP,
			it.SalesRate,
			it.DiscountType,
			it.DiscountValue,
			discAmount,
			it.GSTPercent,
			gstAmount,
			lineTotal,
		)
		if err != nil {
			return 0, "", fmt.Errorf("insert item: %w", err)
		}
	}

	// If INVOICED -> create inventory transactions
	if finalStatus == "INVOICED" {
		for _, it := range in.Items {
			// negative quantity for sale
			_, err = tx.Exec(ctx, `
				INSERT INTO inventory_transactions (
					product_id, quantity, ref_type, ref_id
				) VALUES ($1, $2, $3, $4)
			`,
				it.ProductID,
				-it.Quantity,
				"sale",
				id,
			)
			if err != nil {
				return 0, "", fmt.Errorf("insert inventory transaction: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, "", fmt.Errorf("commit tx: %w", err)
	}

	return id, finalStatus, nil
}

func normalizeDiscountType(items []InvoiceItemInput) string {
	if len(items) == 0 {
		return "INR"
	}
	t := items[0].DiscountType
	if t == "" {
		return "INR"
	}
	return t
}

func GetInvoiceByID(c *gin.Context) {
	idStr := c.Param("id")
	invoiceID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || invoiceID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid invoice id"})
		return
	}

	ctx := c.Request.Context()

	var header struct {
		ID                        int64   `json:"id"`
		InvoiceNumber             string  `json:"invoice_number"`
		CustomerName              string  `json:"customer_name"`
		CustomerMobile            string  `json:"customer_mobile"`
		Status                    string  `json:"status"`
		TotalAmountBeforeDiscount float64 `json:"total_amount_before_discount"`
		TotalDiscount             float64 `json:"total_discount"`
		TaxableAmount             float64 `json:"taxable_amount"`
		TotalGST                  float64 `json:"total_gst"`
		TotalInvoiceAmount        float64 `json:"total_invoice_amount"`
		PaymentMode               string  `json:"payment_mode"`
		CreatedAt                 string  `json:"created_at"`
		InvoicePDFKey             *string `json:"invoice_pdf_key"`
	}

	var createdAt time.Time
	err = db.DB.QueryRow(ctx, `
		SELECT id, invoice_number, customer_name, customer_mobile, status,
		       total_amount_before_discount, total_discount, taxable_amount,
		       total_gst, total_invoice_amount, payment_mode,
		       created_at, invoice_pdf_key
		FROM sales_invoices
		WHERE id = $1 AND deleted_at IS NULL
	`, invoiceID).Scan(
		&header.ID, &header.InvoiceNumber, &header.CustomerName,
		&header.CustomerMobile, &header.Status,
		&header.TotalAmountBeforeDiscount, &header.TotalDiscount,
		&header.TaxableAmount, &header.TotalGST,
		&header.TotalInvoiceAmount, &header.PaymentMode,
		&createdAt, &header.InvoicePDFKey,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			utils.SendErrorResponse(c, http.StatusNotFound, "invoice not found")
			return
		}
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	header.CreatedAt = utils.FormatDateTime(createdAt)

	rows, err := db.DB.Query(ctx, `
		SELECT sii.id, p.name, sii.quantity, sii.sales_rate, sii.discount_amount,
		       sii.gst_percent, sii.gst_amount, sii.line_total
		FROM sales_invoice_items sii
		JOIN products p ON p.id = sii.product_id
		WHERE sii.sales_invoice_id = $1 AND sii.deleted_at IS NULL
	`, invoiceID)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []gin.H{}
	for rows.Next() {
		var it struct {
			ID             int64
			ProductName    string
			Quantity       int
			SalesRate      float64
			DiscountAmount float64
			GSTPercent     float64
			GSTAmount      float64
			LineTotal      float64
		}

		_ = rows.Scan(&it.ID, &it.ProductName, &it.Quantity,
			&it.SalesRate, &it.DiscountAmount, &it.GSTPercent,
			&it.GSTAmount, &it.LineTotal,
		)

		items = append(items, gin.H{
			"id":              it.ID,
			"product_name":    it.ProductName,
			"quantity":        it.Quantity,
			"sales_rate":      it.SalesRate,
			"discount_amount": it.DiscountAmount,
			"gst_percent":     it.GSTPercent,
			"gst_amount":      it.GSTAmount,
			"line_total":      it.LineTotal,
		})
	}

	utils.SendSuccessResponse(c, http.StatusOK, gin.H{
		"invoice": header,
		"items":   items,
	}, "Invoice details fetched successfully")
}

func ListInvoices(c *gin.Context) {
	ctx := c.Request.Context()
	status := c.Query("status")
	from := c.Query("from")
	to := c.Query("to")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset := (page - 1) * limit

	where := "WHERE deleted_at IS NULL"
	params := []interface{}{}
	p := 1

	if status != "" {
		where += " AND status = $" + strconv.Itoa(p)
		params = append(params, status)
		p++
	}

	if from != "" {
		where += " AND created_at >= $" + strconv.Itoa(p)
		params = append(params, from+" 00:00:00")
		p++
	}

	if to != "" {
		where += " AND created_at <= $" + strconv.Itoa(p)
		params = append(params, to+" 23:59:59")
		p++
	}

	query := `
		SELECT id, invoice_number, customer_name, customer_mobile,
		       status, total_invoice_amount, created_at
		FROM sales_invoices
		` + where + `
		ORDER BY created_at DESC
		LIMIT $` + strconv.Itoa(p) + ` OFFSET $` + strconv.Itoa(p+1)

	params = append(params, limit, offset)

	rows, err := db.DB.Query(ctx, query, params...)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	resp := []gin.H{}

	for rows.Next() {
		var r struct {
			ID                 int64
			InvoiceNumber      string
			CustomerName       string
			CustomerMobile     string
			Status             string
			TotalInvoiceAmount float64
			CreatedAt          time.Time
		}

		rows.Scan(
			&r.ID, &r.InvoiceNumber, &r.CustomerName, &r.CustomerMobile,
			&r.Status, &r.TotalInvoiceAmount, &r.CreatedAt,
		)

		resp = append(resp, gin.H{
			"id":                   r.ID,
			"invoice_number":       r.InvoiceNumber,
			"customer_name":        r.CustomerName,
			"customer_mobile":      r.CustomerMobile,
			"status":               r.Status,
			"total_invoice_amount": r.TotalInvoiceAmount,
			"created_at":           utils.FormatDateTime(r.CreatedAt),
		})
	}

	// TODO: Format CreatedAt dates in the loop above if needed, currently returning raw time.Time
	// Let's modify the struct to perform formatting

	utils.SendSuccessResponse(c, http.StatusOK, gin.H{
		"page":     page,
		"limit":    limit,
		"invoices": resp,
	}, "Invoices fetched successfully")
}
