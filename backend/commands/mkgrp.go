package commands

import (
	"fmt"
	"strconv"
	"strings"
)

func ExecuteMkgrp(groupName string) {
	//verificar sesion activa
	if !RequireActiveSession() {
		return
	}

	//validar parametros
	if groupName == "" {
		fmt.Println("Error: el parámetro -name es obligatorio para mkgrp.")
		return
	}

	//obtener sesion actual
	session := GetCurrentSession()
	if session == nil {
		fmt.Println("Error: No se pudo obtener la sesión actual.")
		return
	}

	//verificar que solo root puede crear grupos
	if session.User != "root" {
		fmt.Printf("Error: Solo el usuario 'root' puede crear grupos. Usuario actual: '%s'\n", session.User)
		return
	}

	//buscar la particion montada de la sesion
	mounted := GetMountedPartition(session.PartitionID)
	if mounted == nil {
		fmt.Printf("Error: No se encontró la partición montada con ID '%s'.\n", session.PartitionID)
		return
	}

	//crear el grupo en el archivo users.txt
	err := createGroupInUsersFile(mounted, groupName)
	if err != nil {
		fmt.Printf("Error al crear el grupo '%s': %v\n", groupName, err)
		return
	}

	fmt.Printf("Grupo '%s' creado exitosamente.\n", groupName)
}

// crear grupo en el archivo users.txt
func createGroupInUsersFile(mounted *MountedPartition, groupName string) error {
	currentContent, err := ReadUsersFileContent(mounted)
	if err != nil {
		return fmt.Errorf("error al leer users.txt: %v", err)
	}

	//verificar que el grupo no existe y obtener el siguiente gid
	nextGID, err := validateAndGetNextGID(currentContent, groupName)
	if err != nil {
		return err
	}

	//crear la nueva linea del grupo
	newGroupLine := fmt.Sprintf("%d,G,%s\n", nextGID, groupName)
	newContent := currentContent + newGroupLine

	err = WriteUsersFileContent(mounted, newContent)
	if err != nil {
		return fmt.Errorf("error al escribir users.txt: %v", err)
	}

	fmt.Printf("Linea agregada al users.txt: %d,G,%s\n", nextGID, groupName)
	return nil
}

// validar que el grupo no existe y obtener el siguiente gid
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

			//convertir gid a entero
			gid, err := strconv.Atoi(gidStr)
			if err != nil {
				continue
			}

			//verificar si el grupo ya existe
			if tipo == "G" && gid != 0 && name == groupName {
				return 0, fmt.Errorf("el grupo '%s' ya existe", groupName)
			}

			//actualizar el gid maximo
			if gid > maxGID {
				maxGID = gid
			}
		}
	}

	return maxGID + 1, nil
}
