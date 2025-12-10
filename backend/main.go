package main

import (
    "bufio"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "backend/commands"
)

// Estructuras para la API HTTP
type CommandRequest struct {
    Command string `json:"command"`
}

type CommandResponse struct {
    Success bool   `json:"success"`
    Output  string `json:"output"`
    Error   string `json:"error,omitempty"`
}

func main() {
    // Flag para determinar si ejecutar en modo servidor HTTP o CLI
    serverMode := flag.Bool("server", false, "Ejecutar en modo servidor HTTP")
    port := flag.String("port", "8080", "Puerto para el servidor HTTP")
    flag.Parse()

    if *serverMode {
        startHTTPServer(*port)
    } else {
        startCLI()
    }
}

// Servidor HTTP para el frontend
func startHTTPServer(port string) {
    // Configurar CORS
    http.HandleFunc("/execute", corsMiddleware(executeCommandHandler))
    http.HandleFunc("/health", corsMiddleware(healthHandler))

    fmt.Printf("🚀 Servidor ExtreamFS iniciado en http://localhost:%s\n", port)
    fmt.Println("📡 Esperando comandos desde el frontend...")
    
    log.Fatal(http.ListenAndServe(":"+port, nil))
}

// Middleware para CORS
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Configurar headers CORS
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

        // Responder a OPTIONS request (preflight)
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }

        next(w, r)
    }
}

// Handler para ejecutar comandos
func executeCommandHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != "POST" {
        http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
        return
    }

    var req CommandRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        response := CommandResponse{
            Success: false,
            Error:   "Error al decodificar el comando: " + err.Error(),
        }
        sendJSONResponse(w, response, http.StatusBadRequest)
        return
    }

    // Capturar la salida del comando
    output, err := executeCommandFromHTTP(req.Command)
    
    if err != nil {
        response := CommandResponse{
            Success: false,
            Output:  output,
            Error:   err.Error(),
        }
        sendJSONResponse(w, response, http.StatusOK)
    } else {
        response := CommandResponse{
            Success: true,
            Output:  output,
        }
        sendJSONResponse(w, response, http.StatusOK)
    }
}

// Handler para health check
func healthHandler(w http.ResponseWriter, r *http.Request) {
    response := map[string]string{
        "status":  "ok",
        "message": "Backend ExtreamFS funcionando correctamente",
        "version": "2.0",
    }
    sendJSONResponse(w, response, http.StatusOK)
}

// Enviar respuesta JSON
func sendJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(data)
}

// isComment verifica si una línea es un comentario
func isComment(line string) bool {
    trimmed := strings.TrimSpace(line)
    return strings.HasPrefix(trimmed, "#")
}

// removeInlineComment elimina comentarios inline de una línea
func removeInlineComment(line string) string {
    // Buscar el primer # que no esté dentro de comillas
    inQuotes := false
    quoteChar := byte(0)
    
    for i := 0; i < len(line); i++ {
        char := line[i]
        
        if !inQuotes {
            switch char {
                case '"', '\'':
                    inQuotes = true
                    quoteChar = char
                case '#':
                    // Encontramos un comentario, devolver la parte antes del #
                    return strings.TrimSpace(line[:i])
            }
        } else {
            if char == quoteChar {
                inQuotes = false
                quoteChar = 0
            }
        }
    }
    
    return line
}

// Ejecutar comando desde HTTP y capturar salida
func executeCommandFromHTTP(commandLine string) (string, error) {
    // Redirigir stdout para capturar la salida
    originalStdout := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w

    // Buffer para capturar la salida
    outputChan := make(chan string)
    go func() {
        scanner := bufio.NewScanner(r)
        var output strings.Builder
        for scanner.Scan() {
            output.WriteString(scanner.Text() + "\n")
        }
        outputChan <- output.String()
    }()

    var err error
    
    parts := parseArguments(commandLine)
    if len(parts) == 0 {
        w.Close()
        os.Stdout = originalStdout
        return "", fmt.Errorf("comando vacío")
    }

    command := strings.ToLower(parts[0])
    args := parts[1:]

    err = executeCommand(command, args, commandLine)

    // Restaurar stdout y obtener salida
    w.Close()
    os.Stdout = originalStdout
    output := <-outputChan

    return output, err
}

// Función para ejecutar comandos
func executeCommand(command string, args []string, fullLine string) error {
    switch command {
    case "mkdisk":
        mkdiskCmd := flag.NewFlagSet("mkdisk", flag.ContinueOnError)
        size := mkdiskCmd.Int("size", 0, "Tamaño del disco")
        unit := mkdiskCmd.String("unit", "m", "Unidad del tamaño (K o M).")
        fit := mkdiskCmd.String("fit", "ff", "Tipo de ajuste (BF, FF, WF).")
        path := mkdiskCmd.String("path", "", "Ruta del disco a crear.")

        if err := mkdiskCmd.Parse(args); err != nil {
            return err
        }

        if *path == "" {
            return fmt.Errorf("el parámetro -path es obligatorio para mkdisk")
        }
        if *size <= 0 {
            return fmt.Errorf("el parámetro -size es obligatorio y debe ser positivo")
        }

        commands.ExecuteMkdisk(*size, *unit, *fit, *path)

    case "rmdisk":
        rmdiskCmd := flag.NewFlagSet("rmdisk", flag.ContinueOnError)
        path := rmdiskCmd.String("path", "", "Ruta del disco a eliminar.")

        if err := rmdiskCmd.Parse(args); err != nil {
            return err
        }
        if *path == "" {
            return fmt.Errorf("el parámetro -path es obligatorio para rmdisk")
        }

        commands.ExecuteRmdisk(*path)

    case "fdisk":
        fdiskCmd := flag.NewFlagSet("fdisk", flag.ContinueOnError)
        size := fdiskCmd.Int64("size", 0, "Tamaño de la partición")
        unit := fdiskCmd.String("unit", "m", "Unidad del tamaño (K o M).")
        path := fdiskCmd.String("path", "", "Ruta del disco donde se encuentra la partición.")
        tipo := fdiskCmd.String("type", "primaria", "Tipo de partición (primaria o extendida).")
        fit := fdiskCmd.String("fit", "ff", "Tipo de ajuste (BF, FF, WF).")
        name := fdiskCmd.String("name", "", "Nombre de la partición.")

        if err := fdiskCmd.Parse(args); err != nil {
            return err
        }
        if *path == "" {
            return fmt.Errorf("el parámetro -path es obligatorio para fdisk")
        }
        if *size <= 0 {
            return fmt.Errorf("el parámetro -size es obligatorio y debe ser positivo")
        }

        commands.ExecuteFdisk(*size, *unit, *path, *tipo, *fit, *name)

    case "mount":
        mountCmd := flag.NewFlagSet("mount", flag.ContinueOnError)
        path := mountCmd.String("path", "", "Ruta del disco")
        name := mountCmd.String("name", "", "Nombre de la partición")

        if err := mountCmd.Parse(args); err != nil {
            return err
        }
        if *path == "" {
            return fmt.Errorf("el parámetro -path es obligatorio para mount")
        }
        if *name == "" {
            return fmt.Errorf("el parámetro -name es obligatorio para mount")
        }

        commands.ExecuteMount(*path, *name)

    case "mounted":
        commands.ExecuteMounted()

    case "mkfs":
        mkfsCmd := flag.NewFlagSet("mkfs", flag.ContinueOnError)
        id := mkfsCmd.String("id", "", "ID de la partición montada")
        formatType := mkfsCmd.String("type", "full", "Tipo de formateo (full)")

        if err := mkfsCmd.Parse(args); err != nil {
            return err
        }
        if *id == "" {
            return fmt.Errorf("el parámetro -id es obligatorio para mkfs")
        }

        commands.ExecuteMkfs(*id, *formatType)

    case "login":
        loginCmd := flag.NewFlagSet("login", flag.ContinueOnError)
        user := loginCmd.String("user", "", "Nombre de usuario")
        pass := loginCmd.String("pass", "", "Contraseña del usuario")
        id := loginCmd.String("id", "", "ID de la partición montada")

        if err := loginCmd.Parse(args); err != nil {
            return err
        }
        if *user == "" {
            return fmt.Errorf("el parámetro -user es obligatorio para login")
        }
        if *pass == "" {
            return fmt.Errorf("el parámetro -pass es obligatorio para login")
        }

        commands.ExecuteLogin(*user, *pass, *id)

    case "logout":
        commands.ExecuteLogout()

    case "cat":
        catCmd := flag.NewFlagSet("cat", flag.ContinueOnError)
        files := make(map[string]*string)
        for i := 1; i <= 10; i++ {
            flagName := fmt.Sprintf("file%d", i)
            files[flagName] = catCmd.String(flagName, "", fmt.Sprintf("Archivo %d a mostrar", i))
        }

        if err := catCmd.Parse(args); err != nil {
            return err
        }

        fileMap := make(map[string]string)
        for flagName, flagValue := range files {
            if *flagValue != "" {
                fileMap[flagName] = *flagValue
            }
        }

        if len(fileMap) == 0 {
            return fmt.Errorf("debe especificar al menos un archivo con -file1")
        }

        commands.ExecuteCat(fileMap)

    case "mkgrp":
        mkgrpCmd := flag.NewFlagSet("mkgrp", flag.ContinueOnError)
        groupName := mkgrpCmd.String("name", "", "Nombre del grupo a crear")

        if err := mkgrpCmd.Parse(args); err != nil {
            return err
        }
        if *groupName == "" {
            return fmt.Errorf("el parámetro -name es obligatorio para mkgrp")
        }

        commands.ExecuteMkgrp(*groupName)

    case "rmgrp":
        rmgrpCmd := flag.NewFlagSet("rmgrp", flag.ContinueOnError)
        groupName := rmgrpCmd.String("name", "", "Nombre del grupo a eliminar")

        if err := rmgrpCmd.Parse(args); err != nil {
            return err
        }
        if *groupName == "" {
            return fmt.Errorf("el parámetro -name es obligatorio para rmgrp")
        }

        commands.ExecuteRmgrp(*groupName)

    case "mkusr":
        mkusrCmd := flag.NewFlagSet("mkusr", flag.ContinueOnError)
        username := mkusrCmd.String("user", "", "Nombre del usuario a crear")
        password := mkusrCmd.String("pass", "", "Contraseña del usuario")
        groupName := mkusrCmd.String("grp", "", "Grupo al que pertenece el usuario")

        if err := mkusrCmd.Parse(args); err != nil {
            return err
        }
        if *username == "" || *password == "" || *groupName == "" {
            return fmt.Errorf("los parámetros -user, -pass y -grp son obligatorios para mkusr")
        }

        commands.ExecuteMkusr(*username, *password, *groupName)

    case "rmusr":
        rmusrCmd := flag.NewFlagSet("rmusr", flag.ContinueOnError)
        username := rmusrCmd.String("user", "", "Nombre del usuario a eliminar")

        if err := rmusrCmd.Parse(args); err != nil {
            return err
        }
        if *username == "" {
            return fmt.Errorf("el parámetro -user es obligatorio para rmusr")
        }

        commands.ExecuteRmusr(*username)

    case "chgrp":
        chgrpCmd := flag.NewFlagSet("chgrp", flag.ContinueOnError)
        username := chgrpCmd.String("user", "", "Nombre del usuario")
        groupName := chgrpCmd.String("grp", "", "Nuevo grupo del usuario")

        if err := chgrpCmd.Parse(args); err != nil {
            return err
        }
        if *username == "" || *groupName == "" {
            return fmt.Errorf("los parámetros -user y -grp son obligatorios para chgrp")
        }

        commands.ExecuteChgrp(*username, *groupName)

    case "mkfile":
        mkfileCmd := flag.NewFlagSet("mkfile", flag.ContinueOnError)
        path := mkfileCmd.String("path", "", "Ruta del archivo a crear")
        recursive := mkfileCmd.Bool("r", false, "Crear directorios padre si no existen")
        size := mkfileCmd.Int("size", 0, "Tamaño del archivo en bytes")
        cont := mkfileCmd.String("cont", "", "Archivo del sistema con contenido")

        if err := mkfileCmd.Parse(args); err != nil {
            return err
        }
        if *path == "" {
            return fmt.Errorf("el parámetro -path es obligatorio para mkfile")
        }

        commands.ExecuteMkfile(*path, *recursive, *size, *cont)

    case "mkdir":
        mkdirArgs := parseArguments(fullLine)[1:]
        mkdirCmd := flag.NewFlagSet("mkdir", flag.ContinueOnError)
        path := mkdirCmd.String("path", "", "Ruta del directorio a crear")
        parents := mkdirCmd.Bool("p", false, "Crear directorios padre si no existen")

        if err := mkdirCmd.Parse(mkdirArgs); err != nil {
            return err
        }
        if *path == "" {
            return fmt.Errorf("el parámetro -path es obligatorio para mkdir")
        }

        commands.ExecuteMkdir(*path, *parents)

    case "rep":
        repCmd := flag.NewFlagSet("rep", flag.ContinueOnError)
        name := repCmd.String("name", "", "Nombre del reporte (mbr, disk, inode, block, bm_inode, bm_block, tree, sb, file, ls)")
        path := repCmd.String("path", "", "Ruta donde guardar el reporte")
        id := repCmd.String("id", "", "ID de la partición montada")
        pathFileLs := repCmd.String("path_file_ls", "", "Ruta del archivo o carpeta (para reportes file y ls)")

        if err := repCmd.Parse(args); err != nil {
            return err
        }
        if *name == "" || *path == "" || *id == "" {
            return fmt.Errorf("los parámetros -name, -path e -id son obligatorios para rep")
        }

        commands.ExecuteRep(*name, *path, *id, *pathFileLs)

    default:
        return fmt.Errorf("comando no reconocido: %s", command)
    }

    return nil
}

func startCLI() {
    scanner := bufio.NewScanner(os.Stdin)
    
    for {
        fmt.Print("╰─➤ ")
        if !scanner.Scan() {
            break
        }

        line := scanner.Text()

        if strings.ToLower(line) == "exit" {
            fmt.Println("Exiting...")
            break
        }

        // Verificar si es un comentario
        if isComment(line) {
            fmt.Println("💬 Comentario ignorado")
            continue
        }

        // Remover comentarios inline
        line = removeInlineComment(line)

        // Si después de remover comentarios la línea está vacía, continuar
        if strings.TrimSpace(line) == "" {
            continue
        }

        parts := parseArguments(line)

        if len(parts) == 0 {
            continue
        }

        command := strings.ToLower(parts[0])
        args := parts[1:]

        if err := executeCommand(command, args, line); err != nil {
            fmt.Printf("Error: %v\n", err)
        }
    }

    if err := scanner.Err(); err != nil {
        fmt.Fprintln(os.Stderr, "Error reading input:", err)
    }
}

func parseArguments(line string) []string {
    var args []string
    var current strings.Builder
    inQuotes := false
    quoteChar := byte(0)
    
    for i := 0; i < len(line); i++ {
        char := line[i]
        
        if !inQuotes {
            switch char {
            case '"', '\'':
                inQuotes = true
                quoteChar = char
                current.WriteByte(char) // Incluir la comilla
            case ' ', '\t':
                if current.Len() > 0 {
                    args = append(args, current.String())
                    current.Reset()
                }
            default:
                current.WriteByte(char)
            }
        } else {
            current.WriteByte(char)
            if char == quoteChar {
                inQuotes = false
                quoteChar = 0
            }
        }
    }
    
    if current.Len() > 0 {
        args = append(args, current.String())
    }
    
    return args
}