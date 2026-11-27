package server

import "net/http"

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h handler) { mux.HandleFunc(pattern, a.api(h, false)) }
	adm := func(pattern string, h handler) { mux.HandleFunc(pattern, a.api(h, true)) }

	// session & 2FA
	mux.HandleFunc("POST /api/login", a.hLogin)
	api("POST /api/logout", a.hLogout)
	api("GET /api/me", a.hMe)
	api("POST /api/password", a.hPassword)
	api("POST /api/2fa/setup", a.h2FASetup)
	api("POST /api/2fa/enable", a.h2FAEnable)
	api("POST /api/2fa/disable", a.h2FADisable)
	api("GET /api/users", a.hUsersList)

	// files & recycle bin
	api("GET /api/list", a.hList)
	api("POST /api/mkdir", a.hMkdir)
	api("POST /api/delete", a.hDelete)
	api("POST /api/rename", a.hRename)
	api("POST /api/paste", a.hPaste)
	api("POST /api/save", a.hSaveText)
	api("GET /api/search", a.hSearch)
	api("GET /api/dl", a.hDownload)
	api("GET /api/zip", a.hZip)
	api("GET /api/thumb", a.hThumb)
	api("GET /api/hash", a.hHash)
	api("GET /api/trash", a.hTrashList)
	api("POST /api/trash/restore", a.hTrashRestore)
	api("POST /api/trash/empty", a.hTrashEmpty)

	// uploads
	api("PUT /api/put", a.hPut)
	api("POST /api/upload", a.hUploadInit)
	api("PUT /api/upload/{id}/{idx}", a.hUploadChunk)
	api("GET /api/upload/{id}", a.hUploadStatus)
	api("POST /api/upload/{id}/finish", a.hUploadFinish)
	api("DELETE /api/upload/{id}", a.hUploadAbort)

	// sharing
	api("GET /api/grants", a.hGrantsGet)
	api("POST /api/grants", a.hGrantSet)
	api("DELETE /api/grants/{id}", a.hGrantDelete)
	api("GET /api/links", a.hLinksGet)
	api("POST /api/links", a.hLinkCreate)
	api("DELETE /api/links/{token}", a.hLinkDelete)

	// admin
	adm("GET /api/admin/users", a.hAdminUsers)
	adm("POST /api/admin/users", a.hAdminUserCreate)
	adm("PATCH /api/admin/users/{name}", a.hAdminUserPatch)
	adm("DELETE /api/admin/users/{name}", a.hAdminUserDelete)
	adm("GET /api/admin/mounts", a.hAdminMounts)
	adm("POST /api/admin/mounts", a.hAdminMountSet)
	adm("DELETE /api/admin/mounts/{name}", a.hAdminMountDelete)
	adm("GET /api/admin/browse", a.hAdminBrowse)
	adm("GET /api/admin/grants", a.hAdminGrants)
	adm("GET /api/admin/links", a.hAdminLinks)
	adm("GET /api/admin/audit", a.hAdminAudit)
	adm("GET /api/admin/audit/verify", a.hAdminAuditVerify)
	adm("GET /api/admin/sessions", a.hAdminSessions)
	adm("DELETE /api/admin/sessions/{id}", a.hAdminSessionKick)
	adm("GET /api/admin/status", a.hAdminStatus)
	adm("GET /api/admin/tunnel", a.hAdminTunnelGet)
	adm("POST /api/admin/tunnel", a.hAdminTunnelSet)

	a.publicRoutes(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { a.site.serve(w, r, "index.html") })
	return mux
}

// publicRoutes is everything an anonymous visitor of a share link may reach.
// It is also the ONLY thing served on the Cloudflare Tunnel listener.
func (a *App) publicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/public/{token}/info", a.pub(a.hPubInfo, true))
	mux.HandleFunc("POST /api/public/{token}/unlock", a.pub(a.hPubUnlock, true))
	mux.HandleFunc("GET /api/public/{token}/list", a.pub(a.hPubList, false))
	mux.HandleFunc("GET /api/public/{token}/dl", a.pub(a.hPubDownload, false))
	mux.HandleFunc("GET /api/public/{token}/zip", a.pub(a.hPubZip, false))
	mux.HandleFunc("PUT /api/public/{token}/upload", a.pub(a.hPubUpload, false))
	mux.HandleFunc("GET /s/{token}", func(w http.ResponseWriter, r *http.Request) { a.site.serve(w, r, "share.html") })
	mux.HandleFunc("GET /static/{file}", func(w http.ResponseWriter, r *http.Request) {
		if f := r.PathValue("file"); viaTunnel(r) && f != "share.js" && f != "app.css" && f != "logo.svg" {
			http.NotFound(w, r) // the file-manager UI code is not served to the internet
			return
		}
		a.site.serve(w, r, r.PathValue("file"))
	})
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) { a.site.serve(w, r, "logo.svg") })
}
