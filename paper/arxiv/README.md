# arXiv preprint

`main.tex` is a self-contained LaTeX preprint (article class, inline
bibliography) derived from the author's draft whitepaper in
`docs/cncf-tag-infra/gpu-aware-autoscaling-whitepaper.md`.

## Build

Any standard LaTeX toolchain works; a single `pdflatex` pass is enough because
the bibliography is inline (`thebibliography`).

- Overleaf: upload `main.tex`, compile. (Easiest, no local install.)
- Tectonic: `tectonic main.tex`
- TeX Live: `pdflatex main.tex`

## Submit to arXiv

1. Category: `cs.DC` (Distributed, Parallel, and Cluster Computing).
2. Upload the LaTeX source (`main.tex`); arXiv compiles it server-side.
3. First-time submitters to `cs.DC` may need an endorsement.
4. Keep the author name and ORCID consistent with `CITATION.cff` so citations
   attribute correctly.

The preprint cites the software DOI (`10.5281/zenodo.22866676`); cite the arXiv
ID back from the repository once it is assigned.
