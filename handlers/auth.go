package handlers

import (
	"net/http"
	"time"

	"tulsi-pos/db"
	"tulsi-pos/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret = []byte("YOUR_SUPER_SECRET_KEY") // Replace later with env variable

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func Login(c *gin.Context) {
	var input LoginInput
	if err := c.BindJSON(&input); err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid input")
		return
	}

	// Fetch user
	var (
		userID       int
		passwordHash string
		isActive     bool
	)
	err := db.DB.QueryRow(c, `
		SELECT id, password_hash, is_active
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
	`, input.Email).Scan(&userID, &passwordHash, &isActive)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if !isActive {
		utils.SendErrorResponse(c, http.StatusForbidden, "user is inactive")
		return
	}

	// Check password
	if !utils.CheckPassword(passwordHash, input.Password) {
		utils.SendErrorResponse(c, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Fetch roles
	rows, err := db.DB.Query(c, `
		SELECT r.name
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
	`, userID)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, "cannot fetch roles")
		return
	}

	var roles []string
	for rows.Next() {
		var r string
		rows.Scan(&r)
		roles = append(roles, r)
	}

	// Create JWT token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"roles":   roles,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	})

	tokenString, _ := token.SignedString(jwtSecret)

	utils.SendSuccessResponse(c, http.StatusOK, gin.H{
		"token": tokenString,
		"user": gin.H{
			"id":    userID,
			"email": input.Email,
			"roles": roles,
		},
	}, "Login successful")
}

type RegisterInput struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	Role        string `json:"role"` // admin / cashier
	CreatedByID int    `json:"created_by"`
}

func Register(c *gin.Context) {
	var input RegisterInput
	if err := c.BindJSON(&input); err != nil {
		utils.SendErrorResponse(c, http.StatusBadRequest, "invalid input")
		return
	}

	// Hash password
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, "cannot hash password")
		return
	}

	// Start transaction
	tx, err := db.DB.Begin(c)
	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, "failed to create user")
		return
	}
	defer tx.Rollback(c)

	var userID int
	err = tx.QueryRow(c, `
		INSERT INTO users (name, email, password_hash, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, input.Name, input.Email, hash, input.CreatedByID).Scan(&userID)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Assign role
	_, err = tx.Exec(c, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = $2
	`, userID, input.Role)

	if err != nil {
		utils.SendErrorResponse(c, http.StatusInternalServerError, "invalid role")
		return
	}

	tx.Commit(c)

	utils.SendSuccessResponse(c, http.StatusCreated, gin.H{
		"user_id": userID,
	}, "user created")
}
