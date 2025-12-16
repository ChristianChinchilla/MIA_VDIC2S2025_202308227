package commands

import (
    "fmt"
    "strconv"
    "strings"
)

func ExecuteMkgrp(groupName string) {
    // Verificar sesión activa
    if !RequireActiveSession() {
        return
    }

    // Validar parámetros
    if groupName == "" {
        fmt.Println("Error: el parámetro -name es obligatorio para mkgrp.")
        return
    }

    // Obtener sesión actual
    session := GetCurrentSession()
    if session == nil {
        fmt.Println("Error: No se pudo obtener la sesión actual.")
        return
    }

    // Verificar que solo root puede crear grupos
    if session.User != "root" {
        fmt.Printf("Error: Solo el usuario 'root' puede crear grupos. Usuario actual: '%s'\n", session.User)
        return
    }

    // Buscar la partición montada de la sesión
    mounted := GetMountedPartition(session.PartitionID)
    if mounted == nil {
        fmt.Printf("Error: No se encontró la partición montada con ID '%s'.\n", session.PartitionID)
        return
    }

    // Crear el grupo en el archivo users.txt
    err := createGroupInUsersFile(mounted, groupName)
    if err != nil {
        fmt.Printf("Error al crear el grupo '%s': %v\n", groupName, err)
        return
    }

    fmt.Printf("Grupo '%s' creado exitosamente.\n", groupName)
}

// Crear grupo en el archivo users.txt
func createGroupInUsersFile(mounted *MountedPartition, groupName string) error {
    currentContent, err := ReadUsersFileContent(mounted)
    if err != nil {
        return fmt.Errorf("error al leer users.txt: %v", err)
    }

    // Verificar que el grupo no existe y obtener el siguiente GID
    nextGID, err := validateAndGetNextGID(currentContent, groupName)
    if err != nil {
        return err
    }

    // Crear la nueva línea del grupo
    newGroupLine := fmt.Sprintf("%d,G,%s\n", nextGID, groupName)
    newContent := currentContent + newGroupLine

    err = WriteUsersFileContent(mounted, newContent)
    if err != nil {
        return fmt.Errorf("error al escribir users.txt: %v", err)
    }

    fmt.Printf("Línea agregada al users.txt: %d,G,%s\n", nextGID, groupName)
    return nil
}

// Validar que el grupo no existe y obtener el siguiente GID
func validateAndGetNextGID(content, groupName string) (int, error) {
    lines := strings.Split(content, "\n")
    maxGID := 0
    
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        
        parts := strings.Split(line, ",")
        if len(parts) >= 3 {
            gidStr := strings.TrimSpace(parts[0])
            tipo := strings.TrimSpace(parts[1])
            name := strings.TrimSpace(parts[2])
            
            // Convertir GID a entero
            gid, err := strconv.Atoi(gidStr)
            if err != nil {
                continue // Saltar líneas con GID inválido
            }
            
            // Verificar si el grupo ya existe
            if tipo == "G" && gid != 0 && name == groupName {
                return 0, fmt.Errorf("el grupo '%s' ya existe", groupName)
            }
            
            // Actualizar el GID máximo
            if gid > maxGID {
                maxGID = gid
            }
        }
    }
    
    return maxGID + 1, nil
}