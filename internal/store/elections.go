package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Gerenciador de Eleições (ADR-0012): cada Eleição vive num arquivo SQLite
// próprio dentro da pasta de dados. O log de operações (ADR-0006) é global ao
// banco, então o isolamento por arquivo é o que mantém Desfazer/Restaurar
// escopados a uma Eleição só. A Eleição ativa fica num arquivo meta na pasta.

const activeMetaFile = "eleicao-ativa"

// validDBName: nome simples terminado em .db — barra o path traversal vindo de forms.
var validDBName = regexp.MustCompile(`^[A-Za-z0-9._ -]+\.db$`)

type Elections struct {
	dir string
}

// OpenElections prepara a pasta de dados (cria se não existir).
func OpenElections(dir string) (*Elections, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("pasta de dados: %w", err)
	}
	return &Elections{dir: dir}, nil
}

func (m *Elections) Dir() string { return m.dir }

// Path resolve o caminho de um arquivo de Eleição, validando o nome.
func (m *Elections) Path(file string) (string, error) {
	if !validDBName.MatchString(file) || strings.Contains(file, "..") {
		return "", errors.New("nome de arquivo inválido")
	}
	return filepath.Join(m.dir, file), nil
}

// Active devolve o arquivo da Eleição ativa ("" se o meta não existe ou aponta
// para um arquivo que sumiu).
func (m *Elections) Active() string {
	b, err := os.ReadFile(filepath.Join(m.dir, activeMetaFile))
	if err != nil {
		return ""
	}
	file := strings.TrimSpace(string(b))
	if p, err := m.Path(file); err != nil {
		return ""
	} else if _, err := os.Stat(p); err != nil {
		return ""
	}
	return file
}

// SetActive grava qual Eleição está ativa.
func (m *Elections) SetActive(file string) error {
	if _, err := m.Path(file); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir, activeMetaFile), []byte(file+"\n"), 0o644)
}

// NewFile devolve um nome livre (eleicao-001.db, eleicao-002.db, ...). O nome é
// opaco: o que identifica a Eleição na UI é o nome do congresso dentro do banco.
func (m *Elections) NewFile() string {
	for i := 1; ; i++ {
		name := fmt.Sprintf("eleicao-%03d.db", i)
		if _, err := os.Stat(filepath.Join(m.dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// Delete apaga o arquivo da Eleição e os auxiliares do WAL. Irreversível —
// leva o Histórico junto (a fricção fica na camada web).
func (m *Elections) Delete(file string) error {
	p, err := m.Path(file)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	os.Remove(p + "-wal")
	os.Remove(p + "-shm")
	return nil
}

// ElectionInfo é o retrato de um arquivo de Eleição para a listagem do gerenciador.
type ElectionInfo struct {
	File        string
	Configurada bool // tem congresso (passou do wizard)
	Nome        string
	Ambito      string
	Sociedade   string
	Ano         int
	Aberta      bool // abertura declarada
	Encerrada   bool
	Votantes    int // tamanho do rol
	Operacoes   int
	ModTime     time.Time
	Size        int64
}

// List enumera os *.db da pasta com um retrato de cada um, mais recente primeiro.
// Arquivos ilegíveis entram na lista marcados como não configurados (em vez de
// derrubar a listagem inteira).
func (m *Elections) List(ctx context.Context) ([]ElectionInfo, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, err
	}
	var out []ElectionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") || !validDBName.MatchString(e.Name()) {
			continue
		}
		info := ElectionInfo{File: e.Name()}
		if fi, err := e.Info(); err == nil {
			info.ModTime = fi.ModTime()
			info.Size = fi.Size()
		}
		m.peek(ctx, &info)
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Info devolve o retrato de um único arquivo de Eleição.
func (m *Elections) Info(ctx context.Context, file string) (ElectionInfo, error) {
	p, err := m.Path(file)
	if err != nil {
		return ElectionInfo{}, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return ElectionInfo{}, errors.New("eleição não encontrada")
	}
	info := ElectionInfo{File: file, ModTime: fi.ModTime(), Size: fi.Size()}
	m.peek(ctx, &info)
	return info, nil
}

// peek abre o banco só para leitura do retrato (congresso + contagens) e fecha.
func (m *Elections) peek(ctx context.Context, info *ElectionInfo) {
	p := filepath.Join(m.dir, info.File)
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(2000)", p)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return
	}
	defer db.Close()
	// O nº de operações vale mesmo sem congresso: um banco recém-resetado é
	// "não configurado" mas o Histórico segue lá (e o reset é desfazível).
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation`).Scan(&info.Operacoes)
	var aberta, encerrada int
	err = db.QueryRowContext(ctx,
		`SELECT nome, ambito, sociedade, ano, abertura_declarada, encerrada
		 FROM congress ORDER BY id LIMIT 1`).
		Scan(&info.Nome, &info.Ambito, &info.Sociedade, &info.Ano, &aberta, &encerrada)
	if err != nil {
		return // sem congresso (ou banco antigo/estranho): fica "não configurada"
	}
	info.Configurada = true
	info.Aberta = aberta == 1
	info.Encerrada = encerrada == 1
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM elector`).Scan(&info.Votantes)
}
