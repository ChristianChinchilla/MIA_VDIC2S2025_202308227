package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ExecuteChgrp(username, newGroupName string) {
	// Verificar sesión activa
	if !RequireActiveSession() {
		return
	}

	// Validar parámetros obligatorios
	if username == "" {
		fmt.Println("Error: el parámetro -user es obligatorio para chgrp.")
		return
	}
	if newGroupName == "" {
		fmt.Println("Error: el parámetro -grp es obligatorio para chgrp.")
		return
	}

	// Obtener sesión actual
	session := GetCurrentSession()
	if session == nil {
		fmt.Println("Error: No se pudo obtener la sesión actual.")
		return
	}

	// Verificar que solo root puede cambiar grupos
	if session.User != "root" {
		fmt.Printf("Error: Solo el usuario 'root' puede cambiar grupos. Usuario actual: '%s'\n", session.User)
		return
	}

	// Buscar la partición montada de la sesión
	mounted := GetMountedPartition(session.PartitionID)
	if mounted == nil {
		fmt.Printf("Error: No se encontró la partición montada con ID '%s'.\n", session.PartitionID)
		return
	}

	// Cambiar el grupo del usuario en el archivo users.txt
	err := changeUserGroupInUsersFile(mounted, username, newGroupName)
	if err != nil {
		fmt.Printf("Error al cambiar el grupo del usuario '%s': %v\n", username, err)
		return
	}

	fmt.Printf("Grupo del usuario '%s' cambiado exitosamente a '%s'.\n", username, newGroupName)
}

// Cambiar grupo del usuario en el archivo users.txt
func changeUserGroupInUsersFile(mounted *MountedPartition, username, newGroupName string) error {
	// Abrir el archivo del disco
	file, err := os.OpenFile(mounted.Path, os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("error al abrir el disco: %v", err)
	}
	defer file.Close()

	// Leer el MBR para obtener información de la partición
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		return fmt.Errorf("error al leer el MBR: %v", err)
	}

	// Encontrar la partición específica
	var partition *structs.Partition
	for _, p := range mbr.Mbr_partitions {
		if p.Part_status != '0' {
			partitionNameBytes := p.Part_name[:]
			nullIndex := len(partitionNameBytes)
			for j, b := range partitionNameBytes {
				if b == 0 {
					nullIndex = j
					break
				}
			}
			partitionName := strings.TrimSpace(string(partitionNameBytes[:nullIndex]))

			if strings.EqualFold(partitionName, mounted.Name) || partitionName == mounted.Name {
				partitionCopy := p
				partition = &partitionCopy
				break
			}
		}
	}

	if partition == nil {
		return fmt.Errorf("no se pudo encontrar la partición '%s'", mounted.Name)
	}

	// Leer el superbloque
	file.Seek(partition.Part_start, 0)
	var superblock structs.SuperBloque
	if err := binary.Read(file, binary.LittleEndian, &superblock); err != nil {
		return fmt.Errorf("error al leer el superbloque: %v", err)
	}

	// Leer el contenido actual del archivo users.txt
	currentContent, err := ReadUsersFileContent(mounted)
	if err != nil {
		return fmt.Errorf("error al leer users.txt: %v", err)
	}

	// Validar que el usuario existe y el grupo destino existe
	newGroupGID, err := validateUserAndGroup(currentContent, username, newGroupName)
	if err != nil {
		return err
	}

	newContent, changed, err := changeUserGroup(currentContent, username, newGroupName, newGroupGID)
	if err != nil {
		return err
	}

	if !changed {
		return fmt.Errorf("no se pudo cambiar el grupo del usuario '%s'", username)
	}

	// Escribir el contenido actualizado
	err = WriteUsersFileContent(mounted, newContent)
	if err != nil {
		return fmt.Errorf("error al escribir users.txt: %v", err)
	}

	return nil
}

// Validar que el usuario existe y el grupo destino existe
func validateUserAndGroup(content, username, newGroupName string) (string, error) {
	lines := strings.Split(content, "\n")
	userExists := false
	newGroupGID := ""
	groupExists := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 3 {
			idStr := strings.TrimSpace(parts[0])
			tipo := strings.TrimSpace(parts[1])

			// Convertir ID a entero
			id, err := strconv.Atoi(idStr)
			if err != nil {
				continue // Saltar líneas con ID inválido
			}

			if tipo == "G" && id != 0 { // Es un grupo activo
				groupName := strings.TrimSpace(parts[2])
				if groupName == newGroupName {
					groupExists = true
					newGroupGID = idStr // Obtener el GID del grupo destino
				}
			} else if tipo == "U" && id != 0 { // Es un usuario activo
				if len(parts) >= 4 {
					existingUsername := strings.TrimSpace(parts[3])
					if existingUsername == username {
						userExists = true
					}
				}
			}
		}
	}

	// Verificar que el usuario existe
	if !userExists {
		return "", fmt.Errorf("el usuario '%s' no existe", username)
	}

	// Verificar que el grupo destino existe
	if !groupExists {
		return "", fmt.Errorf("el grupo '%s' no existe o está eliminado", newGroupName)
	}

	// Retornar el GID del grupo destino
	return newGroupGID, nil
}

func changeUserGroup(content, username, newGroupName, newGroupGID string) (string, bool, error) {
	lines := strings.Split(content, "\n")
	var newLines []string
	changed := false

	for _, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)

		if line == "" {
			newLines = append(newLines, originalLine)
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) >= 5 {
			uidStr := strings.TrimSpace(parts[0])
			tipo := strings.TrimSpace(parts[1])
			oldGroupName := strings.TrimSpace(parts[2])
			user := strings.TrimSpace(parts[3])
			password := strings.TrimSpace(parts[4])

			// Convertir UID a entero para validar
			uid, err := strconv.Atoi(uidStr)
			if err != nil {
				newLines = append(newLines, originalLine)
				continue
			}

			// Si es el usuario que buscamos y está activo
			if tipo == "U" && uid != 0 && user == username {
				// Verificar si ya tiene el grupo correcto
				if oldGroupName == newGroupName {
					return "", false, fmt.Errorf("el usuario '%s' ya pertenece al grupo '%s'", username, newGroupName)
				}

				newLine := fmt.Sprintf("%s,U,%s,%s,%s", newGroupGID, newGroupName, user, password)
				newLines = append(newLines, newLine)
				changed = true

				fmt.Printf("Usuario actualizado: %s -> %s\n", originalLine, newLine)
			} else {
				newLines = append(newLines, originalLine)
			}
		} else {
			newLines = append(newLines, originalLine)
		}
	}

	return strings.Join(newLines, "\n"), changed, nil
}
