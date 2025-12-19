import React, { useState, useRef, useEffect } from 'react';
import './App.css';

interface CommandResult {
  command: string;
  output: string;
  timestamp: string;
  isError?: boolean;
}

interface BackendResponse {
  success: boolean;
  output: string;
  error?: string;
}

const App: React.FC = () => {
  const [commands, setCommands] = useState<string>('');
  const [output, setOutput] = useState<CommandResult[]>([]);
  const [isExecuting, setIsExecuting] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [isConnected, setIsConnected] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const outputRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const lineNumbersRef = useRef<HTMLDivElement>(null);

  const BACKEND_URL = 'http://localhost:8080';

  
  useEffect(() => {
    checkBackendConnection();
  }, []);


  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [output]);

  
  const handleTextareaScroll = () => {
    if (textareaRef.current && lineNumbersRef.current) {
      lineNumbersRef.current.scrollTop = textareaRef.current.scrollTop;
    }
  };

  const checkBackendConnection = async () => {
    try {
      const response = await fetch(`${BACKEND_URL}/health`);
      if (response.ok) {
        setIsConnected(true);
        addToOutput('', 'Conectado al backend ExtreamFS', false);
      } else {
        setIsConnected(false);
        addToOutput('', 'Error de conexión con el backend', true);
      }
    } catch (error) {
      setIsConnected(false);
      addToOutput('', 'Backend no disponible. Asegúrate de que esté ejecutándose en puerto 8080', true);
    }
  };

  const handleFileLoad = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

   
    if (!file.name.endsWith('.smia')) {
      addToOutput('', 'Error: Solo se permiten archivos con extensión .smia', true);
      return;
    }

    setIsLoading(true);
    const reader = new FileReader();
    
    reader.onload = (e) => {
      const content = e.target?.result as string;
      setCommands(content);
      addToOutput('', `Archivo "${file.name}" cargado exitosamente`, false);
      setIsLoading(false);
    };

    reader.onerror = () => {
      addToOutput('', 'Error al leer el archivo', true);
      setIsLoading(false);
    };

    reader.readAsText(file);
  };

  const addToOutput = (command: string, result: string, isError: boolean = false) => {
    const timestamp = new Date().toLocaleTimeString();
    setOutput(prev => [...prev, {
      command,
      output: result,
      timestamp,
      isError
    }]);
  };

  const executeCommands = async () => {
    if (!commands.trim()) {
      addToOutput('', 'No hay comandos para ejecutar', true);
      return;
    }

    if (!isConnected) {
      addToOutput('', 'No hay conexión con el backend. Verifica que esté ejecutándose.', true);
      return;
    }

    setIsExecuting(true);
    const commandLines = commands.split('\n').filter(line => line.trim());

    for (const command of commandLines) {
      if (command.trim().startsWith('#') || !command.trim()) continue;

      try {
        addToOutput(command.trim(), 'Ejecutando...', false);
        
        const response = await fetch(`${BACKEND_URL}/execute`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ command: command.trim() }),
        });

        if (response.ok) {
          const result: BackendResponse = await response.json();
          
          setOutput(prev => prev.slice(0, -1));
          
          if (result.success) {
            addToOutput(command.trim(), result.output || 'Comando ejecutado exitosamente', false);
          } else {
            addToOutput(command.trim(), result.error || 'Error desconocido', true);
          }
        } else {
          setOutput(prev => prev.slice(0, -1));
          addToOutput(command.trim(), `Error HTTP: ${response.status} - ${response.statusText}`, true);
        }
      } catch (error) {
        setOutput(prev => prev.slice(0, -1));
        addToOutput(command.trim(), `Error de conexión: ${error}`, true);
      }

     
      await new Promise(resolve => setTimeout(resolve, 300));
    }

    setIsExecuting(false);
  };

  const clearOutput = () => {
    setOutput([]);
  };

  const triggerFileInput = () => {
    fileInputRef.current?.click();
  };

  const reconnectBackend = () => {
    addToOutput('', 'Intentando reconectar...', false);
    checkBackendConnection();
  };

  return (
    <div className="app">
      {/* Header */}
      <header className="header">
        <div className="header-content">
          <h1 className="title">
            ExtreamFS
          </h1>
          <div className="status-indicator">
            <span className="status-text">
              {isExecuting ? 'Ejecutando...' : isConnected ? 'Conectado' : 'Desconectado'}
            </span>
            {!isConnected && (
              <button className="btn btn-sm btn-secondary" onClick={reconnectBackend}>
                Reconectar
              </button>
            )}
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="main-content">
        {/* Left Panel - Commands */}
        <div className="panel commands-panel">
          <div className="panel-header">
            <h2 className="panel-title">
              Comandos
            </h2>
            <div className="panel-actions">
              <button 
                className="btn btn-secondary"
                onClick={triggerFileInput}
                disabled={isLoading}
              >
                {isLoading ? (
                  <>
                    Cargando...
                  </>
                ) : (
                  <>
                    Cargar .smia
                  </>
                )}
              </button>
              <input
                ref={fileInputRef}
                type="file"
                accept=".smia"
                onChange={handleFileLoad}
                style={{ display: 'none' }}
              />
            </div>
          </div>
          
          <div className="textarea-container">
            <textarea
              ref={textareaRef} 
              className="commands-textarea"
              value={commands}
              onChange={(e) => setCommands(e.target.value)}
              onScroll={handleTextareaScroll} 
              placeholder="# Escribe tus comandos aquí..."
              spellCheck={false}
            />
            <div ref={lineNumbersRef} className="line-numbers"> 
              {commands.split('\n').map((_, index) => (
                <div key={index} className="line-number">
                  {index + 1}
                </div>
              ))}
            </div>
          </div>

          <div className="panel-footer">
            <button
              className="btn btn-primary"
              onClick={executeCommands}
              disabled={isExecuting || !commands.trim() || !isConnected}
            >
              {isExecuting ? (
                <>
                  Ejecutando...
                </>
              ) : (
                <>
                  Ejecutar
                </>
              )}
            </button>
          </div>
        </div>

        {/* Right Panel - Output */}
        <div className="panel output-panel">
          <div className="panel-header">
            <h2 className="panel-title">
              Salida del Terminal
            </h2>
            <div className="panel-actions">
              <button 
                className="btn btn-danger"
                onClick={clearOutput}
                disabled={output.length === 0}
              >
                Limpiar
              </button>
            </div>
          </div>

          <div className="output-container" ref={outputRef}>
            {output.length === 0 ? (
              <div className="output-empty">
                <p>No hay salida que mostrar</p>
                <small>Ejecuta algunos comandos para ver los resultados aquí</small>
              </div>
            ) : (
              output.map((result, index) => (
                <div 
                  key={index} 
                  className={`output-entry ${result.isError ? 'error' : 'success'}`}
                >
                  {result.command && (
                    <div className="command-line">
                      <span className="prompt">╰─➤</span>
                      <span className="command">{result.command}</span>
                      <span className="timestamp">{result.timestamp}</span>
                    </div>
                  )}
                  <div className="output-line">
                    <pre>{result.output}</pre>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      </main>
    </div>
  );
};

export default App;