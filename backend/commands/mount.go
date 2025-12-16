package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

// Estructura para manejar particiones montadas en memoria
type MountedPartition struct {
	ID   string
	Path string
	Name string
	Size int64
}

// Variable global para almacenar particiones montadas (simulando RAM)
var mountedPartitions []MountedPartition
var diskCounters = make(map[string]int) // Contador de particiones por disco

func ExecuteMount(path string, name string) {
	if name == "" {
		fmt.Println("Error: el parámetro -name es obligatorio para mount.")
		return
	}

	// Asegurar que el archivo tiene extensión .mia
	if !strings.HasSuffix(strings.ToLower(path), ".mia") {
		path += ".mia"
	}

	// Verificar que el archivo existe
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Error: El archivo '%s' no existe.\n", path)
		return
	}

	// Abrir el archivo del disco EN MODO LECTURA/ESCRITURA
	file, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		fmt.Printf("Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	// Leer el MBR
	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al leer el MBR: %v\n", err)
		return
	}

	// Buscar la partición por nombre
	var foundPartition *structs.Partition
	var partitionIndex int = -1

	for i, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			partitionNameBytes := partition.Part_name[:]
			nullIndex := len(partitionNameBytes)
			for j, b := range partitionNameBytes {
				if b == 0 {
					nullIndex = j
					break
				}
			}
			partitionName := strings.TrimSpace(string(partitionNameBytes[:nullIndex]))

			if strings.EqualFold(partitionName, name) || partitionName == name {
				if partition.Part_type == 'P' {
					partitionCopy := partition
					foundPartition = &partitionCopy
					partitionIndex = i
					break
				} else {
					fmt.Printf("Error: Solo se pueden montar particiones primarias. La partición '%s' es de tipo '%c'.\n", name, partition.Part_type)
					return
				}
			}
		}
	}

	if foundPartition == nil {
		fmt.Printf("Error: No se encontró la partición '%s' en el disco '%s'.\n", name, path)
		return
	}

	// Verificar si la partición ya está montada
	for _, mounted := range mountedPartitions {
		if mounted.Path == path && mounted.Name == name {
			fmt.Printf("Error: La partición '%s' del disco '%s' ya está montada con ID '%s'.\n", name, path, mounted.ID)
			return
		}
	}

	// Generar el ID y correlativo
	id := generatePartitionID(path)
	correlativo := generateCorrelativo()

	// Actualizar la partición en el MBR del disco
	mbr.Mbr_partitions[partitionIndex].Part_correlative = int64(correlativo)
	copy(mbr.Mbr_partitions[partitionIndex].Part_id[:], []byte(id)[:4]) // Solo los primeros 4 bytes

	// Escribir el MBR actualizado de vuelta al disco
	file.Seek(0, 0)
	if err := binary.Write(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al actualizar el MBR: %v\n", err)
		return
	}

	// Crear la entrada de partición montada
	mountedPartition := MountedPartition{
		ID:   id,
		Path: path,
		Name: name,
		Size: foundPartition.Part_s,
	}

	// Agregar a la lista de particiones montadas
	mountedPartitions = append(mountedPartitions, mountedPartition)

	// Mostrar mensaje de éxito
	fmt.Printf("Partición '%s' montada exitosamente.\n", name)
	fmt.Printf("   Disco: %s\n", path)
	fmt.Printf("   ID asignado: %s\n", id)
	fmt.Printf("   Correlativo: %d\n", correlativo)
	fmt.Printf("   Tamaño: %d bytes\n", foundPartition.Part_s)
}

// Generar correlativo secuencial
func generateCorrelativo() int {
	return len(mountedPartitions) + 1
}

func generatePartitionID(diskPath string) string {
	// Últimos dos dígitos del carnet: 202300850 -> 50
	carnetSuffix := "50"

	// Verificar si es el mismo disco o uno nuevo
	partitionNumber, exists := diskCounters[diskPath]
	if !exists {
		// Es un disco nuevo, usar la siguiente letra
		partitionNumber = 1
		diskCounters[diskPath] = partitionNumber
	} else {
		// Es el mismo disco, incrementar el número de partición
		partitionNumber++
		diskCounters[diskPath] = partitionNumber
	}

	// Determinar la letra según el número de discos diferentes montados
	currentLetter := getLetter()

	// Formato: últimos 2 dígitos + número de partición + letra
	return fmt.Sprintf("%s%d%c", carnetSuffix, partitionNumber, currentLetter)
}

func getLetter() byte {
	// Si es el primer disco de esta "serie", usar A
	// Si ya hay discos montados, verificar si necesitamos nueva letra
	uniqueDisks := len(diskCounters)

	if uniqueDisks == 1 {
		return 'A'
	}

	// Para múltiples discos, usar letras consecutivas
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if uniqueDisks-1 < len(letters) {
		return letters[uniqueDisks-1]
	}

	return 'Z' // Fallback si se excede el alfabeto
}

func ExecuteMounted() {
	if len(mountedPartitions) == 0 {
		fmt.Println("No hay particiones montadas.")
		return
	}

	fmt.Println("Particiones montadas:")
	for _, mounted := range mountedPartitions {
		fmt.Printf("ID: %s\n", mounted.ID)
		fmt.Printf("Nombre: %s\n", mounted.Name)
		fmt.Printf("Ruta: %s\n", mounted.Path)
		fmt.Println("-------------------------")
	}
}

// Función para obtener una partición montada por ID (ahora exportada)
func GetMountedPartition(id string) *MountedPartition {
	for i, mounted := range mountedPartitions {
		if mounted.ID == id {
			return &mountedPartitions[i]
		}
	}
	return nil
}
