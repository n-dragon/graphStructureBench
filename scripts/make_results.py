#!/usr/bin/env python3
"""Produit RESULTS.md à partir des CSV de campagne.

Le texte comme les tableaux sont engendrés depuis les mesures : aucun chiffre
n'est saisi à la main, donc aucune affirmation ne peut devenir périmée sans
que la régénération ne la corrige.

    python3 scripts/make_results.py campaigns/2026-09-24 > RESULTS.md
    python3 scripts/make_results.py campaigns/2026-09-24 campaigns/2026-09-22 > RESULTS.md

Avec une seconde campagne en argument, le rapport mesure la reproductibilité
entre les deux, en déduit le seuil en dessous duquel deux structures sont
déclarées ex æquo, et n'affirme un effet que s'il se retrouve dans les deux.
"""
import re
import subprocess
import sys
from datetime import date
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from report import (ORDER, load, sel, uniq, by_structure, table,  # noqa: E402
                    memory_table, throughput_table, scaling_table, latency_table,
                    mib, rate, dur)


# --- accès aux mesures -------------------------------------------------------

def fr(n):
    """Entier à la française : séparateur d'unités par espace insécable fine.

    Un .replace(",", " ") sur une chaîne de prose en mangerait les virgules.
    """
    return f"{n:,}".replace(",", " ")


def pick(rows, **kw):
    """La meilleure mesure (débit maximal) correspondant aux critères."""
    c = sel(rows, **kw)
    return max(c, key=lambda r: r["throughput_qps"]) if c else None


def best_structure(rows, kind, n, workload, threads):
    """La structure au plus fort débit sur cette charge."""
    c = sel(rows, kind=kind, n=n, workload=workload, threads=threads)
    return max(c, key=lambda r: r["throughput_qps"]) if c else None


def smallest_structure(rows, kind, n):
    c = sel(rows, kind=kind, n=n)
    return min(c, key=lambda r: r["index_bytes"]) if c else None


def mem_of(rows, kind, n, structure):
    c = sel(rows, kind=kind, n=n, structure=structure)
    return c[0] if c else None


def speedup(rows, kind, n, workload_a, workload_b, structure, threads):
    """Gain de b sur a, en pourcentage."""
    a = pick(rows, kind=kind, n=n, workload=workload_a, structure=structure, threads=threads)
    b = pick(rows, kind=kind, n=n, workload=workload_b, structure=structure, threads=threads)
    if not a or not b or not a["throughput_qps"]:
        return None
    return 100 * (b["throughput_qps"] / a["throughput_qps"] - 1)


def largest_n(rows, kind):
    ns = uniq(sel(rows, kind=kind), "n")
    return max(ns) if ns else None


# --- reproductibilité entre campagnes ---

def point_key(r):
    return (r["kind"], r["n"], r["structure"], r["workload"], r["threads"], r["queries"])


def compare_runs(cur, prev):
    """Confronte deux campagnes point à point."""
    a = {point_key(r): r for r in prev}
    b = {point_key(r): r for r in cur}
    common = [k for k in b if k in a and a[k]["throughput_qps"]]
    ratios = sorted(b[k]["throughput_qps"] / a[k]["throughput_qps"] for k in common)
    beyond = sum(1 for k in common
                 if not (1 / max(a[k]["spread"], b[k]["spread"])
                         <= b[k]["throughput_qps"] / a[k]["throughput_qps"]
                         <= max(a[k]["spread"], b[k]["spread"])))
    mem = max((abs(b[k]["index_bytes"] - a[k]["index_bytes"]) / a[k]["index_bytes"]
               for k in common if a[k]["index_bytes"]), default=0.0)
    q = lambda p: ratios[int(p * (len(ratios) - 1))]
    return {"n": len(common), "median": q(.5), "q10": q(.1), "q90": q(.9),
            "beyond": beyond, "mem": mem}


def winners_stability(cur, prev):
    def groups(rows):
        g = {}
        for r in rows:
            g.setdefault((r["kind"], r["n"], r["workload"], r["threads"], r["queries"]), []).append(r)
        return g
    go, gn = groups(prev), groups(cur)
    keys = [k for k in go if k in gn]
    same = sum(1 for k in keys
               if max(go[k], key=lambda r: r["throughput_qps"])["structure"] ==
               max(gn[k], key=lambda r: r["throughput_qps"])["structure"])
    return same, len(keys)


def mlp_gradient(rows, threads_max):
    """Gain médian 1→max threads par classe de taille d'index."""
    out = []
    for label, pred in (("en cache", lambda n: n < 50_000),
                        ("~200 000 sommets", lambda n: 50_000 < n < 500_000),
                        ("~1 000 000 sommets", lambda n: n >= 500_000)):
        idx = {}
        for r in rows:
            if pred(r["n"]):
                idx.setdefault((r["kind"], r["n"], r["structure"], r["workload"], r["queries"]),
                               {})[r["threads"]] = r["throughput_qps"]
        g = sorted(d[threads_max] / d[1] for d in idx.values()
                   if 1 in d and threads_max in d and d[1])
        if g:
            out.append((label, g[len(g) // 2]))
    return out


# --- coût de l'abstraction ---------------------------------------------------

def parse_overhead(path):
    """Lit la sortie de BenchmarkCallbackOverhead : {structure: {mode: ns}}."""
    out = {}
    if not Path(path).exists():
        return out
    pattern = re.compile(r"BenchmarkCallbackOverhead/([\w-]+)/(\w+)-\d+\s+\d+\s+([\d.]+) ns/op")
    for line in Path(path).read_text().splitlines():
        m = pattern.search(line)
        if m:
            out.setdefault(m.group(1), {})[m.group(2)] = float(m.group(3))
    return out


def overhead_table(data):
    body = []
    for s in ORDER:
        if s not in data:
            continue
        modes = data[s]
        cb, ba, sl = modes.get("callback"), modes.get("batch"), modes.get("slice")
        gain = f"{100 * (cb / ba - 1):.0f} %" if cb and ba else "-"
        body.append([s,
                     f"{cb:.0f}" if cb else "-",
                     f"{ba:.0f}" if ba else "-",
                     gain,
                     f"{sl:.0f}" if sl else "n/a"])
    return table(["structure", "callback (ns)", "bloc (ns)", "gain du bloc", "slice directe (ns)"], body)


# --- assemblage --------------------------------------------------------------

def section_graph(rows, kind, n, include=("memory", "throughput", "scaling", "latency")):
    out = []
    m = sel(rows, kind=kind, n=n)[0]["m"]
    out.append(f"### {kind} — {fr(n)} sommets, {fr(m)} arêtes")
    if "memory" in include:
        t = memory_table(rows, kind, n)
        if t:
            out.append("\n**Mémoire et construction**\n\n" + t)
    if "throughput" in include:
        res = throughput_table(rows, kind, n,
                               ["bfs", "bfs-batch", "dfs", "neighbors", "neighbors-batch", "hasedge"])
        if res:
            tbl, th = res
            out.append(f"\n**Débit à {th} threads** (requêtes/s, meilleur lot)\n\n" + tbl)
    if "scaling" in include:
        for wl in ("bfs-batch", "hasedge"):
            res = scaling_table(rows, kind, n, wl)
            if res:
                tbl, q = res
                out.append(f"\n**Passage à l'échelle — « {wl} », lot de {fr(q)} requêtes**\n\n" + tbl)
    if "latency" in include:
        res = latency_table(rows, kind, n, "bfs-batch")
        if res:
            tbl, th = res
            out.append(f"\n**Latence d'un parcours complet — « bfs-batch », {th} thread**\n\n" + tbl)
    return "\n".join(out)


def main():
    results_dir = Path(sys.argv[1] if len(sys.argv) > 1 else "results")
    rows = load(results_dir)
    if not rows:
        sys.exit(f"aucun CSV dans {results_dir}/")
    prev_dir = Path(sys.argv[2]) if len(sys.argv) > 2 else None
    prev = load(prev_dir) if prev_dir else []
    cmp_ = compare_runs(rows, prev) if prev else None
    # Seuil d'ex aequo : la demi-largeur de la bande q10-q90 des écarts entre
    # campagnes. Deux structures plus proches que cela ne sont pas
    # départageables sur cette machine. Faute de seconde campagne, on retient
    # une valeur prudente.
    tie = (cmp_["q90"] - cmp_["q10"]) / 2 if cmp_ else 0.10

    go_version = subprocess.run(["go", "version"], capture_output=True, text=True).stdout.split()[2].removeprefix("go")
    cores = subprocess.run(["nproc"], capture_output=True, text=True).stdout.strip()
    commit = subprocess.run(["git", "rev-parse", "--short", "HEAD"], capture_output=True, text=True).stdout.strip()
    kinds = [k for k in ("er", "rmat", "grid") if sel(rows, kind=k)]
    big = {k: largest_n(rows, k) for k in kinds}
    threads_max = max(uniq(rows, "threads"))

    P = []
    w = P.append

    w("# Résultats de campagne\n")
    def campaign_date(d):
        return d.name if re.fullmatch(r"\d{4}-\d{2}-\d{2}", d.name) else date.today().isoformat()

    w(f"Machine : {cores} cœurs, Go {go_version}. Campagne du {campaign_date(results_dir)}"
      + (f", comparée à celle du {campaign_date(prev_dir)}" if prev_dir else "")
      + f" ; rapport engendré au commit `{commit}`.\n")
    w("Ce rapport est **engendré** par `scripts/make_results.py` à partir des CSV produits par "
      "`scripts/run-bench.sh` : les chiffres cités dans le texte sont extraits des mesures, pas "
      f"recopiés. Les mesures brutes sont versionnées dans `{results_dir}/`"
      + (f" et `{prev_dir}/`" if prev_dir else "") + ".\n")

    # --- En bref ---
    w("## En bref\n")
    ref_kind = "er" if "er" in kinds else kinds[0]
    n_ref = big[ref_kind]
    csr = mem_of(rows, ref_kind, n_ref, "csr")
    small = smallest_structure(rows, ref_kind, n_ref)
    def podium(kind, n, workload):
        """Le meilleur, et ceux qui en sont à moins du seuil d'ex aequo."""
        best = {}
        for r in sel(rows, kind=kind, n=n, workload=workload, threads=threads_max):
            if r["structure"] not in best or r["throughput_qps"] > best[r["structure"]]["throughput_qps"]:
                best[r["structure"]] = r
        if not best:
            return "-"
        top = max(best.values(), key=lambda r: r["throughput_qps"])
        tied = sorted((r for r in best.values()
                       if r["throughput_qps"] >= top["throughput_qps"] * (1 - tie)),
                      key=lambda r: -r["throughput_qps"])
        return " ≈ ".join(r["structure"] for r in tied) + f" ({rate(top['throughput_qps'])} req/s)"

    body = []
    for kind in kinds:
        n = big[kind]
        compact = smallest_structure(rows, kind, n)
        body.append([f"{kind} (n={n})", podium(kind, n, "bfs-batch"), podium(kind, n, "hasedge"),
                     f"{compact['structure']} ({compact['index_bytes'] / compact['m']:.2f} o/arête)"
                     if compact else "-"])
    w(table(["graphe", "parcours le plus rapide", "test d'arête le plus rapide", "index le plus compact"], body))
    w(f"\n« ≈ » relie les structures à moins de {100 * tie:.0f} % de la meilleure : "
      + ("c'est l'écart que deux campagnes identiques produisent sur cette machine (§ 7), "
         "elles ne sont donc pas départageables.\n" if cmp_ else
         "seuil prudent, faute de seconde campagne pour mesurer la reproductibilité.\n"))

    # Ce qu'on retient : chaque affirmation est calculée, y compris le choix
    # des structures nommées.
    def best_per_structure(kind, n, workload):
        best = {}
        for r in sel(rows, kind=kind, n=n, workload=workload, threads=threads_max):
            if r["structure"] not in best or r["throughput_qps"] > best[r["structure"]]["throughput_qps"]:
                best[r["structure"]] = r
        return best

    family = None  # structures ex aequo avec la meilleure, sur toutes les charges et topologies
    for kind in kinds:
        for wl in ("bfs-batch", "hasedge", "neighbors-batch"):
            best = best_per_structure(kind, big[kind], wl)
            if not best:
                continue
            top = max(r["throughput_qps"] for r in best.values())
            tied = {st for st, r in best.items() if r["throughput_qps"] >= top * (1 - tie)}
            family = tied if family is None else family & tied
    family = family or set()
    mems = {st: mem_of(rows, ref_kind, n_ref, st) for st in family}
    mems = {st: m for st, m in mems.items() if m}
    lightest = min(mems, key=lambda st: mems[st]["index_bytes"]) if mems else None

    heavy_pool = [r for r in sel(rows, kind=ref_kind, n=n_ref) if r["structure"] != "bitmatrix"]
    heaviest = max(heavy_pool, key=lambda r: r["index_bytes"]) if heavy_pool else None

    def median_rank(structure):
        ranks = []
        groups = {}
        for r in rows:
            groups.setdefault((r["kind"], r["n"], r["workload"], r["threads"], r["queries"]), []).append(r)
        for v in groups.values():
            order = [r["structure"] for r in sorted(v, key=lambda r: -r["throughput_qps"])]
            if structure in order:
                ranks.append((order.index(structure) + 1, len(order)))
        ranks.sort()
        return ranks[len(ranks) // 2] if ranks else None

    parts = []
    if family and lightest:
        others = sorted(family - {lightest})
        parts.append(
            f"**Le choix par défaut est `{lightest}`.** Il fait partie du groupe de tête sur toutes "
            f"les charges et topologies" + (f", à égalité avec {', '.join(f'`{o}`' for o in others)}" if others else "")
            + f" — l'écart de vitesse entre eux reste sous le seuil d'ex æquo. Ce qui le distingue "
            f"est la mémoire : c'est le plus compact du groupe ("
            + ", ".join(f"`{st}` {mems[st]['index_bytes'] / mems[st]['m']:.2f}"
                        for st in sorted(mems, key=lambda st: mems[st]['index_bytes']))
            + " o/arête).")
    if heaviest:
        rk = median_rank(heaviest["structure"])
        parts.append(
            f"`{heaviest['structure']}` est l'index le plus lourd "
            f"({heaviest['index_bytes'] / heaviest['m']:.2f} o/arête, "
            f"{heaviest['index_bytes'] / csr['index_bytes']:.1f} fois le CSR)"
            + (f" et se classe en médiane {rk[0]}e sur {rk[1]} en débit" if rk else "")
            + " : aucun critère ne le justifie ici.")
    v = mem_of(rows, ref_kind, n_ref, "varint-csr")
    vr = pick(rows, kind=ref_kind, n=n_ref, workload="neighbors-batch", structure="varint-csr", threads=threads_max)
    cr = pick(rows, kind=ref_kind, n=n_ref, workload="neighbors-batch", structure="csr", threads=threads_max)
    if v and vr and cr and csr:
        parts.append(
            f"`varint-csr` est la réponse quand la mémoire prime : {100 * (1 - v['index_bytes'] / csr['index_bytes']):.0f} % "
            f"de moins que le CSR sur `{ref_kind}`, pour un débit de lecture d'adjacence de "
            f"{100 * vr['throughput_qps'] / cr['throughput_qps']:.0f} % du sien.")
    w("\n**Ce qu'on retient.** " + " ".join(parts) + "\n")

    # --- Mémoire ---
    w("## 1. Mémoire\n")
    w("Les chiffres ci-dessous sont des deltas de tas mesurés après GC forcé, pas des estimations "
      "— la colonne « analytique » donne le compte déclaré par la structure, pour contrôle.\n")
    if csr and small:
        w(f"Sur le graphe `{ref_kind}` à {fr(n_ref)} sommets"
          f", le CSR occupe {mib(csr['index_bytes'])} Mio, soit {csr['index_bytes'] / csr['m']:.2f} octets "
          f"par arête — la borne théorique est de 4 octets pour la cible plus 4 octets par sommet "
          f"pour les bornes. La structure la plus compacte est `{small['structure']}` avec "
          f"{small['index_bytes'] / small['m']:.2f} o/arête "
          f"({small['index_bytes'] / csr['index_bytes']:.2f}x le CSR).\n")
    pairs = []
    for kind in kinds:
        n = big[kind]
        a = mem_of(rows, kind, n, "adjlist")
        e = mem_of(rows, kind, n, "adjlist-exact")
        if a and e:
            pairs.append((kind, n, a, e))
    if pairs:
        kind, n, a, e = pairs[0]
        w(f"**Le prix d'`append`.** Les deux variantes de liste d'adjacence ne diffèrent que par "
          f"leur construction. Sur `{kind}`, la croissance géométrique coûte "
          f"{a['index_bytes'] / a['m']:.2f} o/arête contre {e['index_bytes'] / e['m']:.2f} pour un "
          f"dimensionnement exact, soit {100 * (a['index_bytes'] / e['index_bytes'] - 1):.0f} % de "
          f"mémoire en trop pour un contenu identique.\n")
    lines = []
    for kind in kinds:
        n = big[kind]
        v, c = mem_of(rows, kind, n, "varint-csr"), mem_of(rows, kind, n, "csr")
        if v and c:
            lines.append(f"- `{kind}` : {v['index_bytes'] / v['m']:.2f} o/arête contre "
                         f"{c['index_bytes'] / c['m']:.2f} en CSR "
                         f"({100 * (1 - v['index_bytes'] / c['index_bytes']):.0f} % de moins)")
    if lines:
        w("**La compression suit la localité.** Le CSR varint encode les écarts entre voisins "
          "consécutifs : plus les identifiants voisins sont proches, moins il consomme.\n")
        w("\n".join(lines) + "\n")

    # --- Parcours ---
    w("## 2. Parcours\n")
    gains = []
    for kind in kinds:
        n = big[kind]
        g = speedup(rows, kind, n, "bfs", "bfs-batch", "csr", threads_max)
        if g is not None:
            gains.append((kind, g))
    if gains:
        degs = {k: sel(rows, kind=k, n=big[k])[0]["m"] / big[k] for k, _ in gains}
        w("**L'accès par bloc, et sa limite.** Même BFS, même structure, seule change la façon de "
          "lire l'adjacence : par callback, ou en remplissant un tampon que la boucle parcourt "
          "ensuite sans indirection. Sur le CSR :\n")
        w("\n".join(f"- `{k}` : {g:+.0f} % de débit (degré moyen {degs[k]:.1f})" for k, g in gains) + "\n")
        losers = [(k, g) for k, g in gains if g < 0]
        winners = [(k, g) for k, g in gains if g >= 0]
        if losers and winners:
            lk, lg = min(losers, key=lambda x: x[1])
            w(f"Le gain n'est donc pas acquis : il vient de l'appel indirect économisé par voisin, "
              f"et il faut le payer en recopiant le bloc. Sur `{lk}`, dont le degré moyen est de "
              f"{degs[lk]:.1f}, la copie n'est amortie sur personne et le bloc perd "
              f"{abs(lg):.0f} %. C'est le compromis à retenir : lire par bloc gagne sur les "
              f"sommets de fort degré, perd sur les graphes de faible degré.\n")
        elif losers:
            w("Sur ces graphes, la recopie du bloc n'est pas amortie par les indirections "
              "économisées.\n")
    lat = []
    for kind in kinds:
        n = big[kind]
        r = pick(rows, kind=kind, n=n, workload="bfs-batch", structure="csr", threads=1)
        if r:
            lat.append((kind, n, r["p50_ns"]))
    if lat:
        w("En latence, un parcours complet mono-thread sur le CSR prend " +
          ", ".join(f"{dur(p)} sur `{k}` ({fr(n)} sommets)" for k, n, p in lat) + ".\n")

    # --- Requêtes ponctuelles ---
    w("## 3. Requêtes ponctuelles\n")
    w("La lecture d'adjacence et le test d'existence d'arête se mesurent par millions de requêtes, "
      "réparties sur les threads par un compteur atomique.\n")
    if True:
        c = pick(rows, kind=ref_kind, n=n_ref, workload="hasedge", structure="csr", threads=threads_max)
        e = pick(rows, kind=ref_kind, n=n_ref, workload="hasedge", structure="edgelist", threads=threads_max)
        if c and e:
            w(f"Le test d'arête sépare nettement les structures : sur `{ref_kind}`, le CSR "
              f"(recherche binaire dans une liste courte) tient {rate(c['throughput_qps'])} req/s "
              f"quand la liste d'arêtes triée (recherche binaire sur les {fr(e['m'])} arêtes) " +
              f"plafonne à {rate(e['throughput_qps'])} req/s — un facteur "
              f"{c['throughput_qps'] / e['throughput_qps']:.1f}.\n")

    # --- Scaling ---
    w("## 4. Passage à l'échelle\n")

    def gains_for(ns):
        out = []
        for kind in kinds:
            for n in ns:
                if not sel(rows, kind=kind, n=n):
                    continue
                for wl in uniq(sel(rows, kind=kind, n=n), "workload"):
                    for st in uniq(sel(rows, kind=kind, n=n, workload=wl), "structure"):
                        for q in uniq(sel(rows, kind=kind, n=n, workload=wl, structure=st), "queries"):
                            a = pick(rows, kind=kind, n=n, workload=wl, structure=st, queries=q, threads=1)
                            b = pick(rows, kind=kind, n=n, workload=wl, structure=st,
                                     queries=q, threads=threads_max)
                            if a and b and a["throughput_qps"]:
                                out.append(b["throughput_qps"] / a["throughput_qps"])
        return sorted(out)

    all_ns = uniq(rows, "n")
    buckets = []
    for n in all_ns:
        c = sel(rows, n=n, structure="csr")
        size = c[0]["index_bytes"] if c else 0
        buckets.append((n, size, gains_for([n])))
    buckets = [b for b in buckets if b[2]]

    allg = gains_for(all_ns)
    if allg:
        w(f"Sur {threads_max} cœurs, le gain médian entre 1 et {threads_max} threads est de "
          f"x{allg[len(allg) // 2]:.2f} sur {len(allg)} points de mesure. Mais la médiane cache "
          f"l'essentiel : le gain **dépend de la taille de l'index**.\n")
        body = []
        for n, size, g in buckets:
            body.append([fr(n), f"{size / (1 << 20):.1f} Mio",
                         f"x{g[len(g) // 2]:.2f}",
                         f"{sum(1 for x in g if x > threads_max)}/{len(g)}"])
        w(table(["sommets", "index csr", "gain médian", f"points au-dessus de x{threads_max}"], body))
        biggest = buckets[-1]
        smallest = buckets[0]
        grad_cur = mlp_gradient(rows, threads_max)
        grad_prev = mlp_gradient(prev, threads_max) if prev else []
        top_cur = grad_cur[-1][1] if grad_cur else 0
        top_prev = grad_prev[-1][1] if grad_prev else None
        rises = bool(grad_cur) and grad_cur[-1][1] > grad_cur[0][1] and (
            not grad_prev or grad_prev[-1][1] > grad_prev[0][1])
        superlinear = top_cur > threads_max * (1 + tie / 2) and (
            top_prev is None or top_prev > threads_max * (1 + tie / 2))
        if rises:
            w(f"\n**Le gain progresse quand l'index sort du cache.** À {fr(smallest[0])} sommets, "
              f"l'index de {smallest[1] / (1 << 20):.1f} Mio tient dans le cache du processeur ; à "
              f"{fr(biggest[0])} sommets, ses {biggest[1] / (1 << 20):.1f} Mio se lisent en mémoire "
              f"vive. Une lecture d'adjacence aléatoire y est limitée par la latence mémoire plutôt "
              f"que par le calcul, et chaque cœur ajouté apporte ses propres défauts de cache en vol "
              f"(parallélisme mémoire).\n")
            if grad_prev:
                w("| gain médian 1 → " + str(threads_max) + " threads | "
                  + " | ".join(l for l, _ in grad_cur) + " |")
                w("| --- | " + " | ".join("---:" for _ in grad_cur) + " |")
                w("| cette campagne | " + " | ".join(f"x{v:.2f}" for _, v in grad_cur) + " |")
                w("| campagne précédente | " + " | ".join(f"x{v:.2f}" for _, v in grad_prev) + " |")
                w("")
            if superlinear:
                w(f"Hors cache, le gain dépasse le nombre de cœurs de façon reproductible "
                  f"(x{top_cur:.2f}" + (f", et x{top_prev:.2f} à la campagne précédente" if top_prev else "")
                  + ").\n")
            else:
                w(f"**Un gain supérieur au nombre de cœurs n'est en revanche pas établi.** Hors "
                  f"cache, cette campagne mesure x{top_cur:.2f}"
                  + (f" et la précédente x{top_prev:.2f}" if top_prev else "")
                  + f" : l'écart est de l'ordre de la variance entre campagnes. La tendance est "
                  f"solide, son amplitude au-delà de x{threads_max} ne l'est pas.\n")
        spreads = sorted(r["spread"] for r in rows if r["threads"] == 1)
        if spreads:
            med_spread = spreads[len(spreads) // 2]
            w(f"S'y ajoute une dispersion réelle : les répétitions d'un même point à 1 thread "
              f"varient de {100 * (med_spread - 1):.0f} % en médiane, jusqu'à "
              f"{100 * (spreads[-1] - 1):.0f} % au pire. La colonne « dispersion » des tableaux "
              f"détaillés donne cet écart point par point : **deux structures qui diffèrent de "
              f"moins que leur dispersion ne sont pas départageables**.\n")
    w("Les index sont immuables après construction : aucune synchronisation n'est nécessaire en "
      "lecture, et le seul état par thread est le tampon de parcours.\n")

    # --- Abstraction ---
    over = parse_overhead(results_dir / "callback-overhead.txt")
    if over:
        w("## 5. Le prix de l'abstraction d'itération\n")
        w("Trois façons de lire l'adjacence d'un sommet tiré au hasard, sur un graphe de 500 000 "
          "sommets. `slice directe` n'est disponible que pour les structures qui stockent leurs "
          "voisins contigus en mémoire : c'est la borne basse, pas une option universelle.\n")
        w(overhead_table(over))
        csr_o = over.get("csr", {})
        if csr_o.get("callback") and csr_o.get("slice"):
            w(f"\nL'écart est considérable : sur le CSR, itérer par callback coûte "
              f"{csr_o['callback'] / csr_o['slice']:.1f} fois le parcours direct de la slice. "
              f"C'est pourquoi le banc mesure les deux modes plutôt que d'en imposer un — sans "
              f"quoi il classerait des interfaces d'itération plutôt que des structures de données.\n")

    # --- Tableaux détaillés ---
    w("## 6. Mesures détaillées\n")
    for kind in kinds:
        for n in uniq(sel(rows, kind=kind), "n"):
            w(section_graph(rows, kind, n))
            w("")

    # --- Reproductibilité ---
    if cmp_:
        same, total = winners_stability(rows, prev)
        w("## 7. Reproductibilité entre campagnes\n")
        w(f"La même campagne, même code de mesure et même machine, a été rejouée le "
          f"{campaign_date(results_dir)} après celle du {campaign_date(prev_dir)}. Sur "
          f"{fr(cmp_['n'])} points de mesure communs :\n")
        w(f"- **la mémoire est reproductible** : écart maximal de {100 * cmp_['mem']:.2f} % ;\n"
          f"- **le débit l'est beaucoup moins** : rapport médian de {cmp_['median']:.3f} entre les "
          f"deux campagnes, mais 80 % des points entre {cmp_['q10']:.2f} et {cmp_['q90']:.2f}, et "
          f"{100 * cmp_['beyond'] / cmp_['n']:.0f} % des points s'écartent de plus que leur propre "
          f"dispersion interne ;\n"
          f"- **la structure la plus rapide d'un point donné** est la même dans {same} cas sur "
          f"{total} seulement.\n")
        w("La dispersion mesurée *au sein* d'une campagne sous-estime donc la variance réelle : "
          "des répétitions rapprochées partagent l'état de la machine, deux campagnes à des jours "
          "d'écart non. D'où les deux règles appliquées dans ce rapport : les structures à moins de "
          f"{100 * tie:.0f} % l'une de l'autre sont déclarées ex æquo, et seul ce qui se retrouve "
          "dans les deux campagnes est présenté comme une conclusion.\n")
        w(f"`python3 scripts/make_results.py {results_dir} {prev_dir}` refait cette comparaison "
          "depuis les mesures versionnées.\n")
        limits_title = "## 8. Ce que ce rapport ne dit pas\n"
    else:
        limits_title = "## 7. Ce que ce rapport ne dit pas\n"

    # --- Limites ---
    w(limits_title)
    w("- **Une seule machine.** Les écarts entre structures inférieurs au seuil d'ex æquo ne "
      "sont pas significatifs ; seuls les ordres de grandeur le sont.\n"
      "- **Graphes synthétiques.** Les topologies sont choisies pour isoler des effets (localité, "
      "hubs, uniformité), pas pour imiter un graphe réel.\n"
      "- **Structures figées.** Rien n'est mesuré après construction : ni insertion, ni "
      "suppression, ni mise à jour.\n"
      "- **Le seuil de promotion des hubs** de la structure hybride est dérivé d'un calcul mémoire "
      "(`deg > n/32`). À un million de sommets, presque aucun sommet ne l'atteint, même en loi de "
      "puissance : la structure se réduit alors à un CSR et ses écarts avec lui sont du bruit.\n"
      "- **Le périmètre complet** est décrit dans `SPEC.md`, les choix de protocole dans "
      "`DECISIONS.md`.\n")

    print("\n".join(P))


if __name__ == "__main__":
    main()
