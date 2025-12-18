package commands

import (
	"fmt"
	"strconv"
	"strings"
)

func ExecuteMkusr(username, password, groupName string) {
	//verificar sesion activa
	if !RequireActiveSession() {
		return
	}

	//validar parametros obligatorios
	if username == "" {
		fmt.Println("Error: el parámetro -user es obligatorio para mkusr.")
		return
	}
	if password == "" {
		fmt.Println("Error: el parámetro -pass es obligatorio para mkusr.")
		return
	}
	if groupName == "" {
		fmt.Println("Error: el parámetro -grp es obligatorio para mkusr.")
		return
	}

	//validar longitud de parametros
	if len(username) > 10 {
		fmt.Printf("Error: el nombre de usuario no puede exceder 10 caracteres. Actual: %d\n", len(username))
		return
	}
	if len(password) > 10 {
		fmt.Printf("Error: la contraseña no puede exceder 10 caracteres. Actual: %d\n", len(password))
		return
	}
	if len(groupName) > 10 {
		fmt.Printf("Error: el nombre del grupo no puede exceder 10 caracteres. Actual: %d\n", len(groupName))
		return
	}

	//obtener sesion actual
	session := GetCurrentSession()
	if session == nil {
		fmt.Println("Error: No se pudo obtener la sesión actual.")
		return
	}

	//verificar que solo root puede crear usuarios
	if session.User != "root" {
		fmt.Printf("Error: Solo el usuario 'root' puede crear usuarios. Usuario actual: '%s'\n", session.User)
		return
	}

	//buscar la particion montada de la sesion
	mounted := GetMountedPartition(session.PartitionID)
	if mounted == nil {
		fmt.Printf("Error: No se encontró la partición montada con ID '%s'.\n", session.PartitionID)
		return
	}

	//crear el usuario en el archivo users.txt
	err := createUserInUsersFile(mounted, username, password, groupName)
	if err != nil {
		fmt.Printf("Error al crear el usuario '%s': %v\n", username, err)
		return
	}

	fmt.Printf("Usuario '%s' creado exitosamente.\n", username)
	fmt.Printf("   Usuario: %s\n", username)
	fmt.Printf("   Grupo: %s\n", groupName)
}

// crear usuario en el archivo users.txt
func createUserInUsersFile(mounted *MountedPartition, username, password, groupName string) error {
	currentContent, err := ReadUsersFileContent(mounted)
	if err != nil {
		return fmt.Errorf("error al leer users.txt: %v", err)
	}

	//validar y obtener informacion necesaria
	groupGID, err := validateAndGetUserInfo(currentContent, username, groupName)
	if err != nil {
		return err
	}

	//crear la nueva linea del usuario
	newUserLine := fmt.Sprintf("%s,U,%s,%s,%s\n", groupGID, groupName, username, password)
	newContent := currentContent + newUserLine

	err = WriteUsersFileContent(mounted, newContent)
	if err != nil {
		return fmt.Errorf("error al escribir users.txt: %v", err)
	}

	fmt.Printf("Linea agregada al users.txt: %s,U,%s,%s,%s\n", groupGID, groupName, username, password)
	return nil
}

// validar que el usuario no existe, el grupo existe y obtener ids
func validateAndGetUserInfo(content, username, groupName string) (string, error) {
	lines := strings.Split(content, "\n")
	groupGID := ""
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

			//convertir id a entero
			id, err := strconv.Atoi(idStr)
			if err != nil {
				continue
			}

			if tipo == "G" && id != 0 {
				name := strings.TrimSpace(parts[2])
				if name == groupName {
					groupExists = true
					groupGID = idStr
					fmt.Printf("Grupo '%s' encontrado con GID=%s\n", groupName, groupGID)
				}
			} else if tipo == "U" && id != 0 {
				if len(parts) >= 4 {
					existingUsername := strings.TrimSpace(parts[3])
					if existingUsername == username {
						return "", fmt.Errorf("el usuario '%s' ya existe", username)
					}
				}
			}
		}
	}

	//verificar que el grupo existe
	if !groupExists {
		return "", fmt.Errorf("el grupo '%s' no existe", groupName)
	}

	//retornar el gid del grupo
	return groupGID, nil
}
