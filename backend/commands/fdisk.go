package commands

import (
	"backend/structs"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func ExecuteFdisk(size int64, unit string, diskName string, tipo string, fit string, name string) {
	// Validar parámetros obligatorios
	if size <= 0 {
		fmt.Printf("Error: El parámetro -size debe ser positivo y mayor a cero.\n")
		return
	}

	if diskName == "" {
		fmt.Printf("Error: El parámetro -diskName es obligatorio.\n")
		return
	}

	if name == "" {
		fmt.Printf("Error: El parámetro -name es obligatorio.\n")
		return
	}

	// Manejar parámetros opcionales con valores por defecto
	if tipo == "" {
		tipo = "P" // Por defecto primaria
	}

	if fit == "" {
		fit = "WF" // Por defecto Worst Fit
	}

	if unit == "" {
		unit = "K" // Por defecto Kilobytes
	}

	// Validar unidades
	unit = strings.ToUpper(unit)
	if unit != "B" && unit != "K" && unit != "M" {
		fmt.Printf("Error: Unidad '%s' no válida. Use 'B', 'K' o 'M'.\n", unit)
		return
	}

	// Validar tipos
	tipo = strings.ToUpper(tipo)
	if tipo != "P" && tipo != "E" && tipo != "L" {
		fmt.Printf("Error: Tipo de partición '%s' no válido. Use 'P', 'E' o 'L'.\n", tipo)
		return
	}

	// Validar fit
	fit = strings.ToUpper(fit)
	if fit != "BF" && fit != "FF" && fit != "WF" {
		fmt.Printf("Error: Ajuste '%s' no válido. Use 'BF', 'FF' o 'WF'.\n", fit)
		return
	}

	if !strings.HasSuffix(strings.ToLower(diskName), ".mia") {
		diskName += ".mia"
	}

	// Construir la ruta completa usando el directorio de discos
	fullPath := filepath.Join(DisksDirectory, diskName)

	// Verificar que el archivo existe
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		fmt.Printf("Error: El archivo '%s' no existe.\n", diskName)
		return
	}

	file, err := os.OpenFile(fullPath, os.O_RDWR, 0644)
	if err != nil {
		fmt.Printf("Error al abrir el archivo: %v\n", err)
		return
	}
	defer file.Close()

	var mbr structs.MBR
	if err := binary.Read(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al leer el MBR: %v\n", err)
		return
	}

	// Convertir tamaño según la unidad
	sizeInBytes := convertSize(size, unit)

	// Validar nombre duplicado
	if err := validatePartitionName(name, &mbr); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Validar restricciones de particiones
	if err := validatePartitionConstraints(tipo, &mbr); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Validar espacio disponible
	if err := validateAvailableSpace(sizeInBytes, &mbr, tipo); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Buscar un slot libre en las particiones (solo para P y E)
	var partitionIndex int = -1
	var startPosition int64

	switch tipo {
	case "P", "E":
		partitionIndex = findFreePartitionSlot(&mbr)
		if partitionIndex == -1 {
			fmt.Printf("Error: No hay slots disponibles para particiones primarias/extendidas.\n")
			return
		}
		startPosition = calculateStartPosition(&mbr, fit, sizeInBytes)
	case "L":
		// Para particiones lógicas, usar la partición extendida
		_, err = handleLogicalPartition(&mbr, sizeInBytes, fit, name, file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		fmt.Printf("Partición lógica '%s' creada exitosamente en '%s'.\n", name, diskName)
		return
	}

	// Crear la nueva partición
	newPartition := structs.NewPartition(
		'1',           // status: activa
		tipo[0],       // tipo: P, E, o L
		fit[0],        // fit: primer carácter de fit
		startPosition, // posición de inicio
		sizeInBytes,   // tamaño en bytes
		[16]byte{},    // nombre (se copia después)
	)

	// Copiar el nombre de la partición
	copy(newPartition.Part_name[:], []byte(name))

	// Asignar la partición al MBR
	mbr.Mbr_partitions[partitionIndex] = newPartition

	// Escribir el MBR actualizado al inicio del archivo
	file.Seek(0, 0)
	if err := binary.Write(file, binary.LittleEndian, &mbr); err != nil {
		fmt.Printf("Error al escribir el MBR: %v\n", err)
		return
	}

	// Crear EBR si es partición extendida
	if tipo == "E" {
		ebr := structs.EBR{
			PartMount: 0,
			PartFit:   fit[0],
			PartStart: startPosition,
			PartS:     sizeInBytes,
			PartNext:  -1,
		}
		copy(ebr.PartName[:], []byte(name))

		// Escribir EBR en la posición de inicio de la partición extendida
		file.Seek(startPosition, 0)
		if err := binary.Write(file, binary.LittleEndian, &ebr); err != nil {
			fmt.Printf("Error al escribir el EBR: %v\n", err)
			return
		}
	}

	var tipoNombre string
	switch tipo {
	case "P":
		tipoNombre = "Primaria"
	case "E":
		tipoNombre = "Extendida"
	case "L":
		tipoNombre = "Lógica"
	}

	fmt.Printf("Partición '%s' de tipo '%s' creada exitosamente en '%s'.\n", name, tipoNombre, diskName)
	fmt.Printf("Tamaño: %d bytes, Ajuste: %s, Posición: %d\n", sizeInBytes, fit, startPosition)

	if err := file.Sync(); err != nil {
		fmt.Printf("Error al sincronizar el archivo: %v\n", err)
		return
	}
}

// Validar nombre duplicado
func validatePartitionName(name string, mbr *structs.MBR) error {
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			partitionName := strings.TrimSpace(string(partition.Part_name[:]))
			if partitionName == name {
				return fmt.Errorf("ya existe una partición con el nombre '%s'", name)
			}
		}
	}
	return nil
}

// Validar restricciones de particiones
func validatePartitionConstraints(tipo string, mbr *structs.MBR) error {
	primaryCount := 0
	extendedCount := 0

	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			switch partition.Part_type {
			case 'P':
				primaryCount++
			case 'E':
				extendedCount++
			}
		}
	}

	// Restricción: máximo 4 particiones primarias + extendidas
	if tipo == "P" || tipo == "E" {
		if primaryCount+extendedCount >= 4 {
			return fmt.Errorf("no se pueden crear más particiones. Máximo 4 particiones primarias/extendidas")
		}
	}

	// Restricción: solo una partición extendida por disco
	if tipo == "E" && extendedCount >= 1 {
		return fmt.Errorf("ya existe una partición extendida en el disco")
	}

	// Restricción: no se puede crear partición lógica sin extendida
	if tipo == "L" && extendedCount == 0 {
		return fmt.Errorf("no se puede crear una partición lógica sin una partición extendida")
	}

	return nil
}

// Validar espacio disponible
func validateAvailableSpace(sizeInBytes int64, mbr *structs.MBR, tipo string) error {
	if tipo == "L" {
		return validateLogicalPartitionSpace(sizeInBytes, mbr)
	}

	// Para particiones primarias/extendidas
	usedSpace := int64(512) // MBR
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			usedSpace += partition.Part_s
		}
	}

	availableSpace := mbr.Mbr_tamano - usedSpace

	if sizeInBytes > availableSpace {
		return fmt.Errorf("no hay espacio suficiente en el disco. Disponible: %d bytes, Requerido: %d bytes",
			availableSpace, sizeInBytes)
	}

	return nil
}

// Validar espacio en partición lógica
func validateLogicalPartitionSpace(sizeInBytes int64, mbr *structs.MBR) error {
	// Encontrar la partición extendida
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' && partition.Part_type == 'E' {
			// Aquí deberías leer los EBRs y calcular el espacio usado
			// Por simplicidad, asumir que hay espacio suficiente
			if sizeInBytes > partition.Part_s-1024 { // Reservar espacio para EBR
				return fmt.Errorf("no hay espacio suficiente en la partición extendida")
			}
			return nil
		}
	}
	return fmt.Errorf("no se encontró partición extendida")
}

// Buscar slot libre
func findFreePartitionSlot(mbr *structs.MBR) int {
	for i := 0; i < 4; i++ {
		if mbr.Mbr_partitions[i].Part_status == '0' {
			return i
		}
	}
	return -1
}

// Función para manejar diferentes ajustes
func calculateStartPosition(mbr *structs.MBR, fit string, sizeNeeded int64) int64 {
	switch fit {
	case "FF": // First Fit
		return calculateFirstFit(mbr, sizeNeeded)
	case "BF": // Best Fit
		return calculateBestFit(mbr, sizeNeeded)
	case "WF": // Worst Fit
		return calculateWorstFit(mbr, sizeNeeded)
	default:
		return calculateWorstFit(mbr, sizeNeeded)
	}
}

// ← IMPLEMENTACIÓN COMPLETA: Best Fit - encuentra el espacio libre más pequeño que sea suficiente
func calculateBestFit(mbr *structs.MBR, sizeNeeded int64) int64 {
	// Obtener todos los espacios libres
	freeSpaces := getFreeSpaces(mbr)

	if len(freeSpaces) == 0 {
		return int64(512) // Después del MBR si no hay particiones
	}

	// Encontrar el espacio libre más pequeño que sea suficiente
	bestStart := int64(-1)
	bestSize := int64(math.MaxInt64)

	for _, space := range freeSpaces {
		if space.Size >= sizeNeeded && space.Size < bestSize {
			bestSize = space.Size
			bestStart = space.Start
		}
	}

	// Si no se encontró espacio suficiente, usar el último espacio disponible
	if bestStart == -1 && len(freeSpaces) > 0 {
		return freeSpaces[len(freeSpaces)-1].Start
	}

	if bestStart == -1 {
		return int64(512)
	}

	return bestStart
}

// ← IMPLEMENTACIÓN COMPLETA: Worst Fit - encuentra el espacio libre más grande
func calculateWorstFit(mbr *structs.MBR, sizeNeeded int64) int64 {
	// Obtener todos los espacios libres
	freeSpaces := getFreeSpaces(mbr)

	if len(freeSpaces) == 0 {
		return int64(512) // Después del MBR si no hay particiones
	}

	// Encontrar el espacio libre más grande
	worstStart := int64(-1)
	worstSize := int64(0)

	for _, space := range freeSpaces {
		if space.Size >= sizeNeeded && space.Size > worstSize {
			worstSize = space.Size
			worstStart = space.Start
		}
	}

	// Si no se encontró espacio suficiente, usar el último espacio disponible
	if worstStart == -1 && len(freeSpaces) > 0 {
		return freeSpaces[len(freeSpaces)-1].Start
	}

	if worstStart == -1 {
		return int64(512)
	}

	return worstStart
}

// ← NUEVA ESTRUCTURA: Para representar espacios libres
type FreeSpace struct {
	Start int64
	Size  int64
}

// ← NUEVA FUNCIÓN: Obtener todos los espacios libres en el disco
func getFreeSpaces(mbr *structs.MBR) []FreeSpace {
	var freeSpaces []FreeSpace
	var usedRanges []FreeSpace

	// Agregar el rango del MBR como usado
	usedRanges = append(usedRanges, FreeSpace{Start: 0, Size: 512})

	// Recopilar todas las particiones existentes
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' {
			usedRanges = append(usedRanges, FreeSpace{
				Start: partition.Part_start,
				Size:  partition.Part_s,
			})
		}
	}

	// Ordenar los rangos usados por posición de inicio
	for i := 0; i < len(usedRanges)-1; i++ {
		for j := i + 1; j < len(usedRanges); j++ {
			if usedRanges[i].Start > usedRanges[j].Start {
				usedRanges[i], usedRanges[j] = usedRanges[j], usedRanges[i]
			}
		}
	}

	// Encontrar espacios libres entre particiones
	currentPos := int64(0)

	for _, used := range usedRanges {
		// Si hay espacio libre antes de esta partición
		if used.Start > currentPos {
			freeSpaces = append(freeSpaces, FreeSpace{
				Start: currentPos,
				Size:  used.Start - currentPos,
			})
		}

		// Actualizar la posición actual al final de esta partición
		endPos := used.Start + used.Size
		if endPos > currentPos {
			currentPos = endPos
		}
	}

	// Agregar espacio libre al final del disco si existe
	if currentPos < mbr.Mbr_tamano {
		freeSpaces = append(freeSpaces, FreeSpace{
			Start: currentPos,
			Size:  mbr.Mbr_tamano - currentPos,
		})
	}

	return freeSpaces
}

// ← MEJORAR: First Fit - encuentra el primer espacio libre suficiente
func calculateFirstFit(mbr *structs.MBR, sizeNeeded int64) int64 {
	// Obtener todos los espacios libres
	freeSpaces := getFreeSpaces(mbr)

	if len(freeSpaces) == 0 {
		return int64(512) // Después del MBR si no hay particiones
	}

	// Encontrar el primer espacio libre que sea suficiente
	for _, space := range freeSpaces {
		if space.Size >= sizeNeeded {
			return space.Start
		}
	}

	// Si no se encontró espacio suficiente, usar el último espacio disponible
	return freeSpaces[len(freeSpaces)-1].Start
}

// ← NUEVA FUNCIÓN: Manejar particiones lógicas
func handleLogicalPartition(mbr *structs.MBR, sizeInBytes int64, fit string, name string, file *os.File) (int64, error) {
	// Encontrar la partición extendida
	var extendedPartition *structs.Partition
	for _, partition := range mbr.Mbr_partitions {
		if partition.Part_status != '0' && partition.Part_type == 'E' {
			extendedPartition = &partition
			break
		}
	}

	if extendedPartition == nil {
		return 0, fmt.Errorf("no se encontró partición extendida")
	}

	// Crear EBR para la partición lógica
	ebrPosition := extendedPartition.Part_start + 1024 // Offset dentro de la extendida

	ebr := structs.EBR{
		PartMount: 0,
		PartFit:   fit[0],
		PartStart: ebrPosition,
		PartS:     sizeInBytes,
		PartNext:  -1, // Última partición lógica por ahora
	}
	copy(ebr.PartName[:], []byte(name))

	// Escribir EBR
	file.Seek(ebrPosition, 0)
	if err := binary.Write(file, binary.LittleEndian, &ebr); err != nil {
		return 0, fmt.Errorf("error escribiendo EBR: %v", err)
	}

	return ebrPosition, nil
}

func convertSize(size int64, unit string) int64 {
	switch strings.ToUpper(unit) {
	case "K":
		return size * 1024
	case "M":
		return size * 1024 * 1024
	case "B":
		return size
	default:
		return size * 1024 // Por defecto Kilobytes
	}
}
