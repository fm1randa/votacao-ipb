package web

// Fluxo do delegado (redesenho 2026-06-09, rascunho Excalidraw do usuário):
// home → "Sou delegado" → login com o token (1x) → Área do delegado, um hub com
// três estados (votação aberta / fechada / já votou) e "Sair". A tela de voto
// deixa de pedir o código — o token vive num cookie de sessão.

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const delegadoCookie = "delegado"

func delegadoToken(r *http.Request) string {
	c, err := r.Cookie(delegadoCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// requireDelegado valida o cookie contra o banco; devolve "" se inválido.
func (s *Server) requireDelegado(ctx context.Context, r *http.Request) string {
	tok := delegadoToken(r)
	if tok == "" {
		return ""
	}
	ok, err := s.st.TokenIssued(ctx, tok)
	if err != nil || !ok {
		return "" // token resetado/revogado → trata como deslogado
	}
	return tok
}

func (s *Server) delegadoLoginForm(w http.ResponseWriter, r *http.Request) {
	if s.requireDelegado(r.Context(), r) != "" {
		http.Redirect(w, r, "/delegado", http.StatusSeeOther)
		return
	}
	s.render(w, "delegado_login.html", map[string]any{"Erro": r.URL.Query().Get("e")})
}

func (s *Server) delegadoLoginSubmit(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	tok := strings.ToUpper(strings.Join(strings.Fields(r.FormValue("token")), ""))
	ok, err := s.st.TokenIssued(r.Context(), tok)
	if err != nil {
		fail(w, err)
		return
	}
	if !ok {
		http.Redirect(w, r, "/delegado/login?e=1", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: delegadoCookie, Value: tok, Path: "/", HttpOnly: true,
		MaxAge: int(12 * time.Hour / time.Second), // cobre o dia do congresso
	})
	http.Redirect(w, r, "/delegado", http.StatusSeeOther)
}

func (s *Server) delegadoLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: delegadoCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// delegadoData monta o estado do hub: "aberta", "fechada" ou "votou".
func (s *Server) delegadoData(ctx context.Context, token string) (map[string]any, error) {
	cong, err := s.st.FirstCongress(ctx)
	if err != nil {
		return nil, err
	}
	data := map[string]any{"Congresso": cong, "Estado": "fechada"}
	round, pos, open, err := s.st.OpenRound(ctx, cong.ID)
	if err != nil {
		return nil, err
	}
	if open {
		voted, err := s.st.HasVoted(ctx, round.ID, token)
		if err != nil {
			return nil, err
		}
		data["Round"], data["Cargo"] = round, pos.Nome
		if voted {
			data["Estado"] = "votou"
		} else {
			data["Estado"] = "aberta"
		}
	}
	return data, nil
}

func (s *Server) delegado(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tok := s.requireDelegado(ctx, r)
	if tok == "" {
		http.Redirect(w, r, "/delegado/login", http.StatusSeeOther)
		return
	}
	data, err := s.delegadoData(ctx, tok)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "delegado.html", data)
}

// delegadoFragment: região viva do hub (muda sozinha quando a mesa abre/encerra).
func (s *Server) delegadoFragment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tok := s.requireDelegado(ctx, r)
	if tok == "" {
		w.Header().Set("HX-Redirect", "/delegado/login")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	data, err := s.delegadoData(ctx, tok)
	if err != nil {
		fail(w, err)
		return
	}
	s.render(w, "delegadoLive", data)
}
