package conventions

// Documentation (DOC1 à DOC5 et DOC8, phase 8.3) : INDEX présent, tableau de l'INDEX, INDEX exhaustif,
// liens morts, nommage de l'arborescence, archives. Les cellules de l'INDEX ne sont jamais gardées (la
// colonne Responsable contient des noms) ; seuls des compteurs entrent dans un résultat.

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// IndexFile est le nom attendu de l'INDEX, dans le dossier de documentation.
const IndexFile = "INDEX.md"

// docsDirNames sont les noms de dossier (en minuscules) où un INDEX hors du dossier configuré est reconnu.
var docsDirNames = []string{"doc", "docs", "documentation", "documentations"}

// Colonnes du tableau de l'INDEX (DOC2), dans l'ordre de la charte, et leurs synonymes normalisés.
var (
	indexColumns  = []string{"document", "theme", "version", "updated", "owner"}
	indexSynonyms = map[string][]string{
		"document": {"document", "documents", "fichier", "fichiers", "file", "files", "nom", "name", "titre",
			"title", "lien", "link", "doc"},
		"theme":   {"theme", "themes", "sujet", "topic", "description", "objet", "categorie", "category", "section"},
		"version": {"version", "versions", "v"},
		"updated": {"derniere mise a jour", "mise a jour", "maj", "date", "last update", "last updated", "updated",
			"modifie le", "date de mise a jour", "derniere modification", "last modified"},
		"owner": {"responsable", "responsables", "owner", "owners", "auteur", "auteurs", "author", "authors",
			"redacteur", "redacteurs", "maintainer", "referent"},
	}
)

var (
	docDirRe  = regexp.MustCompile(`^[0-9]{2}-[a-z0-9]+(-[a-z0-9]+)*$`)
	docFileRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+)+$`)
)

// archiveSuffixes sont les extensions d'archive de DOC8 (comparées en minuscules).
var archiveSuffixes = []string{".zip", ".rar", ".7z", ".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tar.xz"}

// officeSuffixes sont les extensions signalées en information sous la documentation (DOC8).
var officeSuffixes = []string{".docx", ".pdf"}

// findIndex cherche l'INDEX : <docs.dir>/INDEX.md, sinon « index.md » (casse ignorée) dans ce dossier écrit
// à la casse près, sinon dans un dossier doc, docs ou documentation, ou INDEX.md à la racine.
func findIndex(idx *treeIndex, rules FileRules) located {
	exact := rules.DocsDir + "/" + IndexFile
	if idx.files[exact] {
		return located{path: exact}
	}
	var same, other []string
	for _, p := range idx.paths {
		if !strings.EqualFold(path.Base(p), IndexFile) || isVendored(p) {
			continue
		}
		dir := path.Dir(p)
		switch {
		case strings.EqualFold(dir, rules.DocsDir):
			same = append(same, p)
		case dir == "." && path.Base(p) == IndexFile,
			slices.Contains(docsDirNames, strings.ToLower(path.Base(dir))) && strings.Count(p, "/") <= 2:
			other = append(other, p)
		}
	}
	if len(same) > 0 {
		return located{path: pickShallowest(same), code: FileMisnamed}
	}
	if len(other) > 0 {
		return located{path: pickShallowest(other), code: FileElsewhere}
	}
	return located{code: FileAbsent}
}

// docsRoot renvoie le dossier de documentation évalué : celui de l'INDEX trouvé hors de la racine, sinon
// docs.dir.
func docsRoot(index located, rules FileRules) string {
	if index.path != "" && path.Dir(index.path) != "." {
		return path.Dir(index.path)
	}
	return rules.DocsDir
}

// isMediaPath indique un chemin (relatif au dossier de documentation) situé dans le dossier des médias.
func isMediaPath(rel string, rules FileRules) bool {
	first, _, _ := strings.Cut(rel, "/")
	return strings.Contains(rel, "/") && strings.EqualFold(first, rules.MediaDir)
}

// docDocuments renvoie les documents .md du dossier de documentation, hors INDEX et médias, dans l'ordre ;
// limit > 0 en plafonne le nombre.
func docDocuments(idx *treeIndex, root string, rules FileRules, indexPath string, limit int) []string {
	var out []string
	for _, p := range idx.under(root) {
		rel := strings.TrimPrefix(p, root+"/")
		if p == indexPath || isMediaPath(rel, rules) || !strings.EqualFold(path.Ext(p), ".md") {
			continue
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, p)
	}
	return out
}

// CheckDocs mesure DOC1 à DOC5 et DOC8, dans l'ordre d'affichage, et renvoie les listes de chemins.
func CheckDocs(tree FileTree, opts FileOptions) ([]FileCheck, FileLists) {
	index, unreferenced, duplicates := checkIndex(tree, opts)
	rest, lists := CheckDocsTree(tree, opts)
	lists.Unreferenced, lists.DuplicateRows = unreferenced, duplicates
	return append(index, rest...), lists
}

// CheckIndex mesure DOC1 à DOC3 et renvoie les documents qu'aucun lien de l'INDEX ne vise.
func CheckIndex(tree FileTree, opts FileOptions) ([]FileCheck, []string) {
	checks, unreferenced, _ := checkIndex(tree, opts)
	return checks, unreferenced
}

// checkIndex mesure DOC1 à DOC3 ; il renvoie aussi les documents non référencés et ceux en double.
func checkIndex(tree FileTree, opts FileOptions) ([]FileCheck, []string, []string) {
	ids := []string{CriterionDOC1, CriterionDOC2, CriterionDOC3}
	rules := opts.Rules.normalized()
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return unknownAll(ids, reason), nil, nil
	}
	loc := findIndex(idx, rules)
	doc1 := newCheck(CriterionDOC1)
	if loc.code != "" {
		doc1.add(loc.code)
	}
	out := []FileCheck{doc1.done()}
	if loc.path == "" {
		return append(out, unknownAll(ids[1:], UnknownNoFile)...), nil, nil
	}
	data, reason := tree.readBlob(loc.path)
	if reason != "" {
		return append(out, unknownAll(ids[1:], reason)...), nil, nil
	}
	doc := parseMarkdown(data)
	doc3, unreferenced, duplicates := checkDOC3(doc, loc.path, docsRoot(loc, rules), idx, rules, opts.Repo)
	return append(out, checkDOC2(doc), doc3), unreferenced, duplicates
}

// indexColumnsOf associe les cellules d'en-tête aux colonnes attendues : d'abord les égalités exactes avec
// un synonyme, puis les cellules qui en contiennent un ; chaque colonne au plus une fois.
func indexColumnsOf(header []string) map[string]int {
	cols := map[string]int{}
	used := map[int]bool{}
	for _, exact := range []bool{true, false} {
		for i, cell := range header {
			if used[i] {
				continue
			}
			n := normalizeHeading(cell)
			for _, key := range indexColumns {
				if _, ok := cols[key]; ok {
					continue
				}
				match := slices.ContainsFunc(indexSynonyms[key], func(s string) bool {
					if exact {
						return n == s
					}
					return len(s) > 1 && containsPhrase(n, s)
				})
				if match {
					cols[key], used[i] = i, true
					break
				}
			}
		}
	}
	return cols
}

// checkDOC2 : un tableau GFM porte les cinq colonnes attendues et au moins une ligne. Les lignes de tous les
// tableaux complets sont comptées ; sans tableau complet, le meilleur donne les colonnes manquantes.
func checkDOC2(doc mdDoc) FileCheck {
	c := newCheck(CriterionDOC2)
	if len(doc.Tables) == 0 {
		c.add(FileNoTable)
		return c.done()
	}
	best := 0
	for _, t := range doc.Tables {
		cols := indexColumnsOf(t.Header)
		best = max(best, len(cols))
		if len(cols) == len(indexColumns) {
			c.counts[CountTables]++
			c.counts[CountRows] += len(t.Rows)
		}
	}
	c.counts[CountColumns] = best
	switch {
	case best < len(indexColumns):
		c.add(FileMissingColumn)
		c.counts[FileMissingColumn] = len(indexColumns) - best
	case c.counts[CountRows] == 0:
		c.add(FileNoRows)
	}
	return c.done()
}

// checkDOC3 : documents .md de la documentation (hors INDEX et médias) qu'aucun lien de l'INDEX ne vise, et
// documents visés par plusieurs lignes des tableaux complets de l'INDEX (les cinq colonnes de DOC2).
func checkDOC3(doc mdDoc, indexPath, root string, idx *treeIndex, rules FileRules, repo string) (FileCheck,
	[]string, []string) {
	c := newCheck(CriterionDOC3)
	targets := map[string]bool{}
	for _, l := range doc.Links {
		if l.Image {
			continue
		}
		if p, kind := resolveLink(indexPath, l.Target, repo); kind != linkSkip {
			targets[p] = true
		}
	}
	docs := docDocuments(idx, root, rules, indexPath, 0)
	c.counts[CountDocuments] = len(docs)
	var unreferenced []string
	for _, p := range docs {
		if !targets[p] {
			unreferenced = append(unreferenced, p)
			c.addCount(FileUnreferenced)
		}
	}
	rows := map[string]int{}
	for _, t := range doc.Tables {
		if len(indexColumnsOf(t.Header)) < len(indexColumns) {
			continue
		}
		for _, row := range t.Rows {
			seen := map[string]bool{}
			for _, cell := range row {
				for _, target := range linksIn(cell) {
					if p, kind := resolveLink(indexPath, target, repo); kind != linkSkip && idx.files[p] && !seen[p] {
						seen[p] = true
						rows[p]++
					}
				}
			}
		}
	}
	var duplicates []string
	for p, n := range rows {
		if n > 1 {
			duplicates = append(duplicates, p)
			c.addCount(FileDuplicateRow)
		}
	}
	slices.Sort(duplicates)
	return c.done(), unreferenced, duplicates
}

// CheckDocsTree mesure DOC4 (liens morts), DOC5 (nommage) et DOC8 (archives) et renvoie leurs listes.
func CheckDocsTree(tree FileTree, opts FileOptions) ([]FileCheck, FileLists) {
	ids := []string{CriterionDOC4, CriterionDOC5, CriterionDOC8}
	rules := opts.Rules.normalized()
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return unknownAll(ids, reason), FileLists{}
	}
	var lists FileLists
	index := findIndex(idx, rules)
	root := docsRoot(index, rules)
	doc4, dead := checkDOC4(tree, idx, rules, opts.Repo, index, root)
	lists.DeadLinks = dead
	doc5, bad := checkDOC5(idx, rules, root)
	lists.BadNames = bad
	doc8, archives, office := checkDOC8(idx, rules, root)
	lists.Archives, lists.OfficeDocuments = archives, office
	return []FileCheck{doc4, doc5, doc8}, lists
}

// checkDOC4 : liens relatifs (hors images) du README, de l'INDEX et des documents dont la cible n'existe
// pas ; une cible présente avec une autre casse est signalée comme telle.
func checkDOC4(tree FileTree, idx *treeIndex, rules FileRules, repo string, index located, root string) (FileCheck,
	[]DeadLink) {
	c := newCheck(CriterionDOC4)
	var sources []string
	if readme := findReadme(idx); readme.path != "" {
		sources = append(sources, readme.path)
	}
	if index.path != "" {
		sources = append(sources, index.path)
	}
	all := docDocuments(idx, root, rules, index.path, 0)
	if len(all) > MaxDocuments {
		c.counts[CountUnread] += len(all) - MaxDocuments
		all = all[:MaxDocuments]
	}
	sources = append(sources, all...)
	var dead []DeadLink
	seen := map[string]bool{}
	for _, src := range sources {
		if seen[src] {
			continue
		}
		seen[src] = true
		data, reason := tree.readBlob(src)
		if reason != "" {
			c.counts[CountUnread]++
			continue
		}
		c.counts[CountSources]++
		for _, l := range parseMarkdown(data).Links {
			if l.Image {
				continue
			}
			p, kind := resolveLink(src, l.Target, repo)
			if kind != linkRelative {
				continue
			}
			c.counts[CountLinks]++
			if idx.exists(p) {
				continue
			}
			d := DeadLink{Source: src, Target: l.Target, Resolved: p, CaseMismatch: idx.existsFold(p)}
			if d.CaseMismatch {
				c.addCount(FileCaseMismatch)
			} else {
				c.addCount(FileDeadLink)
			}
			dead = append(dead, d)
		}
	}
	fc := c.done()
	if fc.Status == CriterionConform && c.counts[CountUnread] > 0 {
		fc.Status, fc.Reason = CriterionUnknown, UnknownUnreadDocuments
	}
	return fc, dead
}

// checkDOC5 : sous le dossier de documentation, dossiers « NN-thème » (sauf le dossier des médias et son
// contenu) et fichiers en minuscules à tirets (sauf INDEX.md, README.md et les fichiers cachés).
func checkDOC5(idx *treeIndex, rules FileRules, root string) (FileCheck, []string) {
	if !idx.dirs[root] {
		return unknownCheck(CriterionDOC5, UnknownNoDocsDir), nil
	}
	c := newCheck(CriterionDOC5)
	var bad []string
	dirs := map[string]bool{}
	for _, p := range idx.under(root) {
		rel := strings.TrimPrefix(p, root+"/")
		segs := strings.Split(rel, "/")
		for i := range segs[:len(segs)-1] {
			d := strings.Join(segs[:i+1], "/")
			if dirs[d] {
				continue
			}
			dirs[d] = true
			if i == 0 && strings.EqualFold(segs[0], rules.MediaDir) || isMediaPath(d, rules) {
				continue
			}
			c.counts[CountDirs]++
			if !docDirRe.MatchString(segs[i]) {
				c.addCount(FileDirName)
				bad = append(bad, root+"/"+d+"/")
			}
		}
		name := segs[len(segs)-1]
		if name == IndexFile || name == ReadmeFile || strings.HasPrefix(name, ".") {
			continue
		}
		c.counts[CountFiles]++
		if !docFileRe.MatchString(name) {
			c.addCount(FileFileName)
			bad = append(bad, p)
		}
	}
	slices.Sort(bad)
	return c.done(), bad
}

// checkDOC8 : archives dans tout le dépôt (ou sous la documentation seulement, selon docs.archives_scope) ;
// .docx et .pdf sous la documentation en information.
func checkDOC8(idx *treeIndex, rules FileRules, root string) (FileCheck, []string, []string) {
	c := newCheck(CriterionDOC8)
	var archives, office []string
	for _, p := range idx.paths {
		lower := strings.ToLower(p)
		inDocs := strings.HasPrefix(p, root+"/")
		if (rules.ArchivesScope == ArchivesScopeRepository || inDocs) &&
			slices.ContainsFunc(archiveSuffixes, func(s string) bool { return strings.HasSuffix(lower, s) }) {
			archives = append(archives, p)
			c.addCount(FileArchive)
		}
		if inDocs && slices.ContainsFunc(officeSuffixes, func(s string) bool { return strings.HasSuffix(lower, s) }) {
			office = append(office, p)
			c.addCount(FileOfficeDocument)
		}
	}
	return c.done(), archives, office
}
