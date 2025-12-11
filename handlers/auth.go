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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
		return
	}

	if !isActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "user is inactive"})
		return
	}

	// Check password
	if !utils.CheckPassword(passwordHash, input.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot fetch roles"})
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

	c.JSON(200, gin.H{
		"token": tokenString,
		"user": gin.H{
			"id":    userID,
			"email": input.Email,
			"roles": roles,
		},
	})
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
		c.JSON(400, gin.H{"error": "invalid input"})
		return
	}

	// Hash password
	hash, err := utils.HashPassword(input.Password)
	if err != nil {
		c.JSON(500, gin.H{"error": "cannot hash password"})
		return
	}

	// Start transaction
	tx, err := db.DB.Begin(c)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to create user"})
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
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Assign role
	_, err = tx.Exec(c, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = $2
	`, userID, input.Role)

	if err != nil {
		c.JSON(500, gin.H{"error": "invalid role"})
		return
	}

	tx.Commit(c)

	c.JSON(201, gin.H{
		"message": "user created",
		"user_id": userID,
	})
}
