package types

import (
	"time"

	"antelope/models"
)

type EmailDto struct {
	Email string `json:"email" binding:"required,nonblank"`
}

type RegisterDto struct {
	Email    string `json:"email" binding:"required,nonblank"`
	Password string `json:"password" binding:"required,nonblank"`
	Code     string `json:"code" binding:"required,nonblank"`
}

type ResetPasswordDto struct {
	Email    string `json:"email" binding:"required,nonblank"`
	Password string `json:"password" binding:"required,nonblank"`
	Code     string `json:"code" binding:"required,nonblank"`
}

type LoginDto struct {
	Email    string `json:"email" binding:"required,nonblank"`
	Password string `json:"password" binding:"required,nonblank"`
}

type UserListDto struct {
	ID           uint      `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	Department   string    `json:"department"`
	Group        string    `json:"group"`
	UpdatedAt    time.Time `json:"updated_at"`
	Status       int       `json:"status"`
	Role         string    `json:"role"`
	AuthSource   string    `json:"auth_source"`
	AuthProvider string    `json:"auth_provider"`
}

type UpdateTokenDto struct {
	RefreshToken string `json:"refreshToken" binding:"required,nonblank"`
}

type UserAddDto struct {
	Name       string `json:"name" binding:"required,nonblank"`
	Email      string `json:"email" binding:"required,nonblank"`
	Department string `json:"department" binding:"required"`
	Group      string `json:"group" binding:"required"`
	Status     int    `json:"status" binding:"min=0,max=1"`
	Role       string `json:"role" binding:"required,nonblank"`
	Password   string `json:"password" binding:"required,nonblank"`
}

// ToUser change to models.User for DB operations
func (u *UserAddDto) ToUser() models.User {
	return models.User{
		Name:       u.Name,
		Email:      u.Email,
		Department: u.Department,
		Group:      u.Group,
		Status:     u.Status,
		Role:       u.Role,
		Password:   u.Password,
	}
}

type UserEditDto struct {
	Name       string `json:"name" binding:"required,nonblank"`
	Email      string `json:"email" binding:"required,nonblank"`
	Department string `json:"department" binding:"required"`
	Group      string `json:"group" binding:"required"`
	Status     int    `json:"status" binding:"min=0,max=1"`
	Role       string `json:"role" binding:"required,nonblank"`
}

func (u *UserEditDto) ToUser() models.User {
	return models.User{
		Name:       u.Name,
		Email:      u.Email,
		Department: u.Department,
		Group:      u.Group,
		Status:     u.Status,
		Role:       u.Role,
	}
}

// UserProfileDto is the self-service profile returned to the current user.
type UserProfileDto struct {
	ID           uint   `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Department   string `json:"department"`
	Group        string `json:"group"`
	Role         string `json:"role"`
	AuthSource   string `json:"auth_source"`
	AuthProvider string `json:"auth_provider"`
}

// UserUpdateProfileDto carries the fields a user may edit on their own profile.
type UserUpdateProfileDto struct {
	Name       string `json:"name" binding:"required,nonblank"`
	Department string `json:"department"`
}

// ChangePasswordDto carries a self-service password change request.
type ChangePasswordDto struct {
	OldPassword string `json:"old_password" binding:"required,nonblank"`
	NewPassword string `json:"new_password" binding:"required,nonblank"`
}
