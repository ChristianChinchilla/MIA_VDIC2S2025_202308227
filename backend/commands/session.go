package commands

import (
    "fmt"
)

// Estructura para manejar la sesión activa
type Session struct {
    User        string
    Group       string
    PartitionID string
    IsActive    bool
}

// Variable global para la sesión activa
var currentSession *Session

// Iniciar sesión
func StartSession(user, group, partitionID string) {
    currentSession = &Session{
        User:        user,
        Group:       group,
        PartitionID: partitionID,
        IsActive:    true,
    }
}

// Cerrar sesión
func EndSession() {
    currentSession = nil
}

// Verificar si hay sesión activa
func IsSessionActive() bool {
    return currentSession != nil && currentSession.IsActive
}

// Obtener sesión actual
func GetCurrentSession() *Session {
    if currentSession != nil && currentSession.IsActive {
        return currentSession
    }
    return nil
}

// Verificar sesión y mostrar error si no existe
func RequireActiveSession() bool {
    if !IsSessionActive() {
        fmt.Println("Error: No existe una sesión activa. Use el comando 'login' para iniciar sesión.")
        return false
    }
    return true
}

// Mostrar información de la sesión actual
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