package handlers

import (
	"net/http"
	"strconv"
	"time"

	"tulsi-pos/db"
	"tulsi-pos/utils"

	"github.com/gin-gonic/gin"
)

type Supplier struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name" binding:"required"`
	ContactInfo string    `json:"contact_info"`
	CreatedAt   time.Time `json:"created_at"`
}

// POST /suppliers
func CreateSupplier(c *gin.Context) {
	var s Supplier
	if err := c.ShouldBindJSON(&s); err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid input")
		return
	}

	var id int64
	err := db.DB.QueryRow(c.Request.Context(), `
		INSERT INTO suppliers (name, contact_info)
		VALUES ($1, $2)
		RETURNING id
	`, s.Name, s.ContactInfo).Scan(&id)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	utils.SendSuccessResponse(c, http.StatusCreated, gin.H{
		"id": id,
	}, "supplier created")
}

// GET /suppliers
func GetSuppliers(c *gin.Context) {
	rows, err := db.DB.Query(c.Request.Context(), `
		SELECT id, name, contact_info, created_at
		FROM suppliers
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	suppliers := []Supplier{}
	for rows.Next() {
		var s Supplier
		if err := rows.Scan(&s.ID, &s.Name, &s.ContactInfo, &s.CreatedAt); err != nil {
			continue
		}
		suppliers = append(suppliers, s)
	}

	utils.SendSuccessResponse(c, http.StatusOK, suppliers, "Suppliers fetched successfully")
}

// GET /suppliers/:id
func GetSupplierByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid id")
		return
	}

	var s Supplier
	err = db.DB.QueryRow(c.Request.Context(), `
		SELECT id, name, contact_info, created_at
		FROM suppliers
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(&s.ID, &s.Name, &s.ContactInfo, &s.CreatedAt)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusNotFound, "supplier not found")
		return
	}

	utils.SendSuccessResponse(c, http.StatusOK, s, "Supplier details fetched successfully")
}

// PUT /suppliers/:id
func UpdateSupplier(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid id")
		return
	}

	var s Supplier
	if err := c.ShouldBindJSON(&s); err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid input")
		return
	}

	res, err := db.DB.Exec(c.Request.Context(), `
		UPDATE suppliers
		SET name = $1, contact_info = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, s.Name, s.ContactInfo, id)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	if res.RowsAffected() == 0 {
		utils.SendErrorResponse(c, http.StatusNotFound, "supplier not found")
		return
	}

	utils.SendSuccessResponse(c, http.StatusOK, nil, "supplier updated")
}

// DELETE /suppliers/:id
func DeleteSupplier(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid id")
		return
	}

	res, err := db.DB.Exec(c.Request.Context(), `
		UPDATE suppliers
		SET deleted_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	if res.RowsAffected() == 0 {
		utils.SendErrorResponse(c, http.StatusNotFound, "supplier not found")
		return
	}

	utils.SendSuccessResponse(c, http.StatusOK, nil, "supplier deleted")
}
