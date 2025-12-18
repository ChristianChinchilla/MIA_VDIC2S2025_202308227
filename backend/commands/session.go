package commands

import (
	"fmt"
)

// estructura para manejar la sesion activa
type Session struct {
	User        string
	Group       string
	PartitionID string
	IsActive    bool
}

// variable global para la sesion activa
var currentSession *Session

// iniciar sesion
func StartSession(user, group, partitionID string) {
	currentSession = &Session{
		User:        user,
		Group:       group,
		PartitionID: partitionID,
		IsActive:    true,
	}
}

// cerrar sesion
func EndSession() {
	currentSession = nil
}

// verificar si hay sesion activa
func IsSessionActive() bool {
	return currentSession != nil && currentSession.IsActive
}

// obtener sesion actual
func GetCurrentSession() *Session {
	if currentSession != nil && currentSession.IsActive {
		return currentSession
	}
	return nil
}

// verificar sesion y mostrar error si no existe
func RequireActiveSession() bool {
	if !IsSessionActive() {
		fmt.Println("Error: No existe una sesión activa. Use el comando 'login' para iniciar sesión.")
		return false
	}
	return true
}

// mostrar informacion de la sesion actual
func ShowCurrentSession() {
	if session := GetCurrentSession(); session != nil {
		fmt.Printf("Sesión activa:\n")
		fmt.Printf("   Usuario: %s\n", session.User)
		fmt.Printf("   Grupo: %s\n", session.Group)
		fmt.Printf("   Partición: %s\n", session.PartitionID)
	} else {
		fmt.Println("No hay sesión activa.")
	}
}
