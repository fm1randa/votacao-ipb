// Ponte C do servidor Go para o app iOS (ADR-0015).
//
// O iOS não permite exec() de binários, então o truque do Android (servidor
// como processo filho) não se aplica: aqui o MESMO bootstrap de sempre
// (internal/cli.Run, com os mesmos argumentos -addr/-host/-data) roda numa
// goroutine dentro do processo do app. Compilado com -buildmode=c-archive pelo
// ios/build-go.sh e empacotado como VotacaoGo.xcframework.
//
// Convenção das strings: tudo que devolve *C.char aloca com malloc — quem
// chama libera com VotacaoLiberar.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"votacao-ipb/internal/cli"
)

// main é exigido pelo -buildmode=c-archive, mas nunca roda.
func main() {}

const maxLinhas = 500

var (
	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
	ultErro string

	logs = &anel{}
)

// anel é o ring buffer dos logs visíveis na aba "Logs" do app: linhas do
// servidor Go + eventos do app (espelha o HostLog do Android).
type anel struct {
	mu      sync.Mutex
	linhas  []string
	parcial bytes.Buffer
}

func (a *anel) add(origem, linha string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.linhas) >= maxLinhas {
		a.linhas = a.linhas[1:]
	}
	a.linhas = append(a.linhas, time.Now().Format("15:04:05")+" ["+origem+"] "+linha)
}

// Write recebe a saída do pacote log (uma ou mais linhas por chamada).
func (a *anel) Write(p []byte) (int, error) {
	a.mu.Lock()
	a.parcial.Write(p)
	var prontas []string
	for {
		i := bytes.IndexByte(a.parcial.Bytes(), '\n')
		if i < 0 {
			break
		}
		prontas = append(prontas, string(a.parcial.Next(i+1)[:i]))
	}
	a.mu.Unlock()
	for _, l := range prontas {
		a.add("go", l)
	}
	return len(p), nil
}

func (a *anel) texto() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Join(a.linhas, "\n")
}

func init() {
	log.SetFlags(0) // o anel já carimba a hora
	log.SetOutput(io.MultiWriter(os.Stderr, logs))
}

// VotacaoIniciar sobe o servidor com os argumentos de linha de comando
// separados por '\n' (ex.: "-addr\n:8090\n-host\n172.20.10.1\n-data\n/pasta").
// Retorna 0 se iniciou, 1 se já estava rodando. O resultado da subida (porta
// ocupada, banco ilegível…) aparece em VotacaoErro e nos logs.
//
//export VotacaoIniciar
func VotacaoIniciar(args *C.char) C.int {
	mu.Lock()
	defer mu.Unlock()
	if done != nil {
		select {
		case <-done: // a execução anterior terminou sozinha (erro)
		default:
			return 1
		}
	}
	var argv []string
	if s := C.GoString(args); s != "" {
		argv = strings.Split(s, "\n")
	}
	stop, done, ultErro = make(chan struct{}), make(chan struct{}), ""
	go func(stop chan struct{}, done chan struct{}) {
		defer close(done)
		defer func() { // pânico no servidor não pode derrubar o app hospedeiro
			if r := recover(); r != nil {
				registraErro(errors.New("pânico no servidor"))
				log.Printf("pânico: %v", r)
			}
		}()
		if err := cli.Run(argv, stop); err != nil {
			registraErro(err)
			log.Printf("ERRO: %v", err)
		}
	}(stop, done)
	return 0
}

func registraErro(err error) {
	mu.Lock()
	ultErro = err.Error()
	mu.Unlock()
}

// VotacaoParar pede o encerramento e espera o servidor terminar (até ~6s).
//
//export VotacaoParar
func VotacaoParar() {
	mu.Lock()
	s, d := stop, done
	if s != nil {
		select {
		case <-s:
		default:
			close(s)
		}
	}
	mu.Unlock()
	if d != nil {
		select {
		case <-d:
		case <-time.After(6 * time.Second):
		}
	}
}

// VotacaoRodando: 1 enquanto a goroutine do servidor está viva.
//
//export VotacaoRodando
func VotacaoRodando() C.int {
	mu.Lock()
	d := done
	mu.Unlock()
	if d == nil {
		return 0
	}
	select {
	case <-d:
		return 0
	default:
		return 1
	}
}

// VotacaoErro devolve o último erro do servidor ("" se nenhum).
//
//export VotacaoErro
func VotacaoErro() *C.char {
	mu.Lock()
	defer mu.Unlock()
	return C.CString(ultErro)
}

// VotacaoLogs devolve o conteúdo do ring buffer de logs.
//
//export VotacaoLogs
func VotacaoLogs() *C.char { return C.CString(logs.texto()) }

// VotacaoLog registra um evento do app (origem "app") no mesmo buffer.
//
//export VotacaoLog
func VotacaoLog(linha *C.char) { logs.add("app", C.GoString(linha)) }

// VotacaoLiberar libera uma string devolvida por esta ponte.
//
//export VotacaoLiberar
func VotacaoLiberar(p *C.char) { C.free(unsafe.Pointer(p)) }
