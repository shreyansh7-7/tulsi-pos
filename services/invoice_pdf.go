package services

import (
	"bytes"
	"context"
	"fmt"
	"time"

	awsclient "tulsi-pos/aws"
	"tulsi-pos/db"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jung-kurt/gofpdf"
)

type InvoiceHeader struct {
	ID                 int64
	InvoiceNumber      string
	CustomerName       string
	CustomerMobile     string
	TotalInvoiceAmount float64
}

type InvoiceItem struct {
	ProductName string
	Quantity    int
	SalesRate   float64
	LineTotal   float64
}

func GenerateAndUploadInvoicePDF(ctx context.Context, invoiceID int64) (string, error) {
	if awsclient.S3 == nil {
		return "", fmt.Errorf("S3 client not initialized")
	}
	if awsclient.InvoiceBucket == "" {
		return "", fmt.Errorf("invoice bucket not configured")
	}

	// 1) Load header
	var h InvoiceHeader
	err := db.DB.QueryRow(ctx, `
		SELECT id, invoice_number, customer_name, customer_mobile, total_invoice_amount
		FROM sales_invoices
		WHERE id = $1 AND deleted_at IS NULL
	`, invoiceID).Scan(&h.ID, &h.InvoiceNumber, &h.CustomerName, &h.CustomerMobile, &h.TotalInvoiceAmount)
	if err != nil {
		return "", fmt.Errorf("load invoice header: %w", err)
	}

	// 2) Load items
	rows, err := db.DB.Query(ctx, `
		SELECT p.name, sii.quantity, sii.sales_rate, sii.line_total
		FROM sales_invoice_items sii
		JOIN products p ON p.id = sii.product_id
		WHERE sii.sales_invoice_id = $1 AND sii.deleted_at IS NULL
	`, invoiceID)
	if err != nil {
		return "", fmt.Errorf("load invoice items: %w", err)
	}
	defer rows.Close()

	var items []InvoiceItem
	for rows.Next() {
		var it InvoiceItem
		if err := rows.Scan(&it.ProductName, &it.Quantity, &it.SalesRate, &it.LineTotal); err != nil {
			return "", err
		}
		items = append(items, it)
	}

	// 3) Generate PDF in memory
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, "Tulsi POS Invoice")

	pdf.Ln(12)
	pdf.SetFont("Arial", "", 11)
	pdf.Cell(40, 6, fmt.Sprintf("Invoice: %s", h.InvoiceNumber))
	pdf.Ln(5)
	pdf.Cell(40, 6, fmt.Sprintf("Customer: %s", h.CustomerName))
	pdf.Ln(5)
	pdf.Cell(40, 6, fmt.Sprintf("Mobile: %s", h.CustomerMobile))
	pdf.Ln(10)

	pdf.SetFont("Arial", "B", 11)
	pdf.Cell(80, 6, "Product")
	pdf.Cell(20, 6, "Qty")
	pdf.Cell(30, 6, "Rate")
	pdf.Cell(30, 6, "Total")
	pdf.Ln(7)

	pdf.SetFont("Arial", "", 10)
	for _, it := range items {
		pdf.Cell(80, 5, it.ProductName)
		pdf.Cell(20, 5, fmt.Sprintf("%d", it.Quantity))
		pdf.Cell(30, 5, fmt.Sprintf("%.2f", it.SalesRate))
		pdf.Cell(30, 5, fmt.Sprintf("%.2f", it.LineTotal))
		pdf.Ln(5)
	}

	pdf.Ln(10)
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 6, fmt.Sprintf("Total: %.2f", h.TotalInvoiceAmount))

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return "", fmt.Errorf("generate pdf: %w", err)
	}

	// 4) Upload to S3
	key := fmt.Sprintf("invoices/%s/%s.pdf",
		time.Now().Format("2006-01-02"),
		h.InvoiceNumber,
	)

	_, err = awsclient.S3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(awsclient.InvoiceBucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String("application/pdf"),
	})
	if err != nil {
		return "", fmt.Errorf("upload to s3: %w", err)
	}

	// 5) Save key in DB
	_, err = db.DB.Exec(ctx, `
		UPDATE sales_invoices
		SET invoice_pdf_key = $1
		WHERE id = $2
	`, key, invoiceID)
	if err != nil {
		return "", fmt.Errorf("update invoice: %w", err)
	}

	return key, nil
}
