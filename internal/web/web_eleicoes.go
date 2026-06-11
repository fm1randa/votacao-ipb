package web

// Gerenciador de Eleições (ADR-0012): um arquivo SQLite por Eleição numa pasta
// de dados. Aqui ficam a listagem e as ações criar/abrir/resetar/excluir.
// Resetar é restaurável (vira Operação no Histórico da própria Eleição);
// Excluir apaga o arquivo — irreversível. Os dois exigem digitar o nome exato.

import (
	"net/http"
	"strings"

	"votacao-ipb/internal/store"
)

func (s *Server) eleicoes(w http.ResponseWriter, r *http.Request) {
	list, err := s.mgr.List(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "eleicoes.html", map[string]any{
		"Active":   "eleicoes",
		"Eleicoes": list,
		"Ativa":    s.mgr.Active(),
		"Erro":     r.URL.Query().Get("e"),
	})
}

// eleicaoCriar cria um banco novo, propaga o PIN da Mesa (um PIN só) e o torna
// ativo — o wizard assume do passo do congresso em diante.
func (s *Server) eleicaoCriar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hash, err := s.db().PINHash(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	file := s.mgr.NewFile()
	p, err := s.mgr.Path(file)
	if err != nil {
		fail(w, err)
		return
	}
	st, err := store.Open(p)
	if err != nil {
		fail(w, err)
		return
	}
	if err := st.SetPINHash(ctx, hash); err != nil {
		st.Close()
		fail(w, err)
		return
	}
	st.Close()
	if err := s.switchTo(file); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/board/setup", http.StatusSeeOther)
}

func (s *Server) eleicaoAbrir(w http.ResponseWriter, r *http.Request) {
	file := r.FormValue("file")
	if _, err := s.mgr.Info(r.Context(), file); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.switchTo(file); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/board", http.StatusSeeOther)
}

// confirmaNome aplica a fricção: para Eleições configuradas, o digitado precisa
// bater exatamente com o nome de EXIBIÇÃO ("UMP da IPB Cordovil") — é o que a
// Mesa vê na lista, e cobre a Nacional, cujo campo nome é vazio. Não
// configuradas (vazias) dispensam — não há nada a perder nem nome a digitar.
func confirmaNome(info store.ElectionInfo, digitado string) bool {
	if !info.Configurada {
		return true
	}
	return strings.TrimSpace(digitado) == entidadeNome(info.Ambito, info.Sociedade, info.Nome)
}

func (s *Server) eleicaoResetar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	file := r.FormValue("file")
	info, err := s.mgr.Info(ctx, file)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if !confirmaNome(info, r.FormValue("confirm")) {
		http.Error(w, "o nome digitado não confere", 400)
		return
	}
	if file == s.mgr.Active() {
		err = s.db().ResetEleicao(ctx)
		s.congCache.Store(congEntry{}) // o vocabulário volta ao default
	} else {
		var p string
		if p, err = s.mgr.Path(file); err == nil {
			var st *store.Store
			if st, err = store.Open(p); err == nil {
				err = st.ResetEleicao(ctx)
				st.Close()
			}
		}
	}
	if err != nil {
		fail(w, err)
		return
	}
	s.actionDone(w, r, "/board/eleicoes", "Eleição resetada — desfazível pelo Histórico.", false)
}

func (s *Server) eleicaoExcluir(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	file := r.FormValue("file")
	info, err := s.mgr.Info(ctx, file)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if !confirmaNome(info, r.FormValue("confirm")) {
		http.Error(w, "o nome digitado não confere", 400)
		return
	}
	// Excluindo a ativa: troca para outra Eleição antes (ou cria uma vazia,
	// preservando o PIN — o sistema sempre tem uma Eleição ativa).
	if file == s.mgr.Active() {
		hash, _ := s.db().PINHash(ctx)
		var destino string
		list, _ := s.mgr.List(ctx)
		for _, e := range list {
			if e.File != file {
				destino = e.File
				break
			}
		}
		if destino == "" {
			destino = s.mgr.NewFile()
			p, err := s.mgr.Path(destino)
			if err != nil {
				fail(w, err)
				return
			}
			st, err := store.Open(p)
			if err != nil {
				fail(w, err)
				return
			}
			st.SetPINHash(ctx, hash)
			st.Close()
		}
		if err := s.switchTo(destino); err != nil {
			fail(w, err)
			return
		}
	}
	if err := s.mgr.Delete(file); err != nil {
		fail(w, err)
		return
	}
	s.actionDone(w, r, "/board/eleicoes", "Eleição excluída definitivamente.", false)
}
