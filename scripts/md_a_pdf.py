#!/usr/bin/env python3
"""Convierte la documentacion Markdown del taller a PDF.

Uso:
    python scripts/md_a_pdf.py docs/documentacion.md docs/documentacion.pdf

Cubre lo que usa la documentacion: encabezados, parrafos con negrita/cursiva/
codigo, listas, citas, tablas de tuberias, reglas horizontales y bloques de
codigo cercados (los diagramas ASCII se componen en monoespaciada ajustando el
tamano para que quepan a lo ancho de la pagina).

Requiere: pip install reportlab
"""
import html
import os
import re
import sys

from reportlab.lib import colors
from reportlab.lib.enums import TA_JUSTIFY
from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import inch
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (HRFlowable, KeepTogether, ListFlowable, ListItem, Paragraph,
                                Preformatted, SimpleDocTemplate, Spacer, Table, TableStyle)

MARGEN = 0.7 * inch
ANCHO_UTIL = letter[0] - 2 * MARGEN

# --- Fuentes -----------------------------------------------------------------
# Las fuentes base del formato PDF (Helvetica/Courier) no incluyen los
# caracteres de dibujo de cajas de los diagramas ni simbolos como ≤ o →. Se
# intenta incrustar DejaVu (Unicode completo y de licencia libre); si no esta
# disponible, se usan las fuentes base y se transliteran esos caracteres.
RUTAS_FUENTES = [
    "/usr/share/fonts/truetype/dejavu",
    "/usr/share/fonts/TTF",
    "/usr/local/share/fonts",
    "/Library/Fonts",
    os.path.join(os.environ.get("WINDIR", "C:/Windows"), "Fonts"),
]

TRANSLITERACION = {
    "\u2500": "-", "\u2501": "-", "\u2550": "=", "\u2502": "|", "\u2503": "|", "\u2551": "|",
    "\u250c": "+", "\u2510": "+", "\u2514": "+", "\u2518": "+", "\u251c": "+", "\u2524": "+",
    "\u252c": "+", "\u2534": "+", "\u253c": "+", "\u2554": "+", "\u2557": "+", "\u255a": "+",
    "\u255d": "+", "\u2560": "+", "\u2563": "+", "\u2566": "+", "\u2569": "+", "\u256c": "+",
    "\u25ba": ">", "\u25c4": "<", "\u25b2": "^", "\u25bc": "v", "\u2022": "*",
    "\u2192": "->", "\u2190": "<-", "\u2264": "<=", "\u2265": ">=", "\u00b7": "-",
    "\u2026": "...", "\u201c": '"', "\u201d": '"', "\u2018": "'", "\u2019": "'",
    "\u2013": "-", "\u2014": "--",
}
TABLA_TRANSLITERACION = str.maketrans(TRANSLITERACION)


def registrar_fuentes():
    """Registra DejaVu si esta disponible.

    Devuelve (texto, negrita, cursiva, monoespaciada, hay_unicode).
    """
    archivos = {
        "DejaVu": "DejaVuSans.ttf",
        "DejaVu-Bold": "DejaVuSans-Bold.ttf",
        "DejaVu-Oblique": "DejaVuSans-Oblique.ttf",
        "DejaVuMono": "DejaVuSansMono.ttf",
    }
    encontradas = {}
    for carpeta in RUTAS_FUENTES:
        for nombre, archivo in archivos.items():
            ruta = os.path.join(carpeta, archivo)
            if nombre not in encontradas and os.path.exists(ruta):
                encontradas[nombre] = ruta
    if len(encontradas) == len(archivos):
        for nombre, ruta in encontradas.items():
            pdfmetrics.registerFont(TTFont(nombre, ruta))
        return "DejaVu", "DejaVu-Bold", "DejaVu-Oblique", "DejaVuMono", True
    return "Helvetica", "Helvetica-Bold", "Helvetica-Oblique", "Courier", False


F_TEXTO, F_NEGRITA, F_CURSIVA, F_MONO, HAY_UNICODE = registrar_fuentes()


def normalizar(texto):
    """Adapta el texto a los caracteres que la fuente disponible sabe dibujar."""
    return texto if HAY_UNICODE else texto.translate(TABLA_TRANSLITERACION)


hojas = getSampleStyleSheet()
ESTILOS = {
    "cuerpo": ParagraphStyle("cuerpo", parent=hojas["BodyText"], fontName=F_TEXTO,
                             fontSize=9.5, leading=13, alignment=TA_JUSTIFY, spaceAfter=6),
    "h1": ParagraphStyle("h1", parent=hojas["Heading1"], fontName=F_NEGRITA, fontSize=18,
                         leading=22, spaceBefore=6, spaceAfter=10,
                         textColor=colors.HexColor("#0F2E4C")),
    "h2": ParagraphStyle("h2", parent=hojas["Heading2"], fontName=F_NEGRITA, fontSize=14,
                         leading=18, spaceBefore=14, spaceAfter=8,
                         textColor=colors.HexColor("#14507F")),
    "h3": ParagraphStyle("h3", parent=hojas["Heading3"], fontName=F_NEGRITA, fontSize=11.5,
                         leading=15, spaceBefore=10, spaceAfter=6,
                         textColor=colors.HexColor("#1F6FB2")),
    "h4": ParagraphStyle("h4", parent=hojas["Heading4"], fontName=F_NEGRITA, fontSize=10.5,
                         leading=14, spaceBefore=8, spaceAfter=4),
    "cita": ParagraphStyle("cita", parent=hojas["BodyText"], fontName=F_CURSIVA, fontSize=10,
                           leading=14, leftIndent=16, spaceAfter=8,
                           textColor=colors.HexColor("#333333")),
    "celda": ParagraphStyle("celda", parent=hojas["BodyText"], fontName=F_TEXTO, fontSize=7.6,
                            leading=9.6, spaceAfter=0),
    "celda_encabezado": ParagraphStyle("celda_encabezado", parent=hojas["BodyText"],
                                       fontName=F_NEGRITA, fontSize=7.8, leading=9.8,
                                       spaceAfter=0, textColor=colors.white),
    "lista": ParagraphStyle("lista", parent=hojas["BodyText"], fontName=F_TEXTO, fontSize=9.5,
                            leading=13, spaceAfter=3),
}

PATRON_INLINE = re.compile(r"(\*\*.+?\*\*|__.+?__|`[^`]+`|\*[^*\n]+\*|_[^_\n]+_|\[[^\]]+\]\([^)]+\))")


def a_marcado(texto):
    """Traduce el formato en linea de Markdown al mini-HTML de reportlab."""
    salida = []
    for trozo in PATRON_INLINE.split(texto):
        if not trozo:
            continue
        if trozo.startswith("**") and trozo.endswith("**") and len(trozo) > 4:
            salida.append("<b>" + html.escape(trozo[2:-2]) + "</b>")
        elif trozo.startswith("__") and trozo.endswith("__") and len(trozo) > 4:
            salida.append("<b>" + html.escape(trozo[2:-2]) + "</b>")
        elif trozo.startswith("`") and trozo.endswith("`") and len(trozo) > 2:
            salida.append('<font face="' + F_MONO + '" color="#B03030">'
                          + html.escape(trozo[1:-1]) + "</font>")
        elif trozo.startswith("[") and "](" in trozo:
            salida.append('<font color="#1F4E79"><u>'
                          + html.escape(trozo[1:trozo.index("](")]) + "</u></font>")
        elif len(trozo) > 2 and trozo[0] in "*_" and trozo[-1] == trozo[0]:
            salida.append("<i>" + html.escape(trozo[1:-1]) + "</i>")
        else:
            salida.append(html.escape(trozo))
    return normalizar("".join(salida))


def es_separador_de_tabla(linea):
    return bool(re.match(r"^\s*\|[\s:\-|]+\|\s*$", linea))


def celdas(linea):
    return [c.strip() for c in linea.strip().strip("|").split("|")]


def construir_tabla(filas):
    encabezado = celdas(filas[0])
    cuerpo = [celdas(f) for f in filas[2:]]
    n = len(encabezado)

    # Ancho proporcional al contenido mas largo de cada columna, acotado.
    anchos_texto = []
    for i in range(n):
        largo = len(encabezado[i])
        for fila in cuerpo:
            if i < len(fila):
                largo = max(largo, min(len(fila[i]), 90))
        anchos_texto.append(max(largo, 6))
    total = float(sum(anchos_texto))
    anchos = [ANCHO_UTIL * a / total for a in anchos_texto]

    datos = [[Paragraph(a_marcado(c), ESTILOS["celda_encabezado"]) for c in encabezado]]
    for fila in cuerpo:
        fila = (fila + [""] * n)[:n]
        datos.append([Paragraph(a_marcado(c), ESTILOS["celda"]) for c in fila])

    tabla = Table(datos, colWidths=anchos, repeatRows=1, hAlign="LEFT")
    tabla.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), colors.HexColor("#14507F")),
        ("GRID", (0, 0), (-1, -1), 0.4, colors.HexColor("#9AA5B1")),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 4),
        ("RIGHTPADDING", (0, 0), (-1, -1), 4),
        ("TOPPADDING", (0, 0), (-1, -1), 3),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 3),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.HexColor("#F3F6F9")]),
    ]))
    return tabla


def construir_codigo(lineas):
    lineas = [normalizar(l) for l in lineas]
    ancho_max = max((len(l) for l in lineas), default=1)
    # Las monoespaciadas usadas ocupan ~0.602 em por caracter: se ajusta el
    # tamano de letra para que el diagrama mas ancho quepa en la pagina.
    tamano = min(8.0, (ANCHO_UTIL - 14) / (0.602 * max(ancho_max, 1)))
    tamano = max(tamano, 3.6)
    estilo = ParagraphStyle("codigo", fontName=F_MONO, fontSize=tamano,
                            leading=tamano * 1.22, leftIndent=6,
                            textColor=colors.HexColor("#1A1A1A"),
                            backColor=colors.HexColor("#F2F4F6"), borderPadding=4)
    return Preformatted("\n".join(lineas) if lineas else " ", estilo)


def convertir(ruta_md, ruta_pdf):
    lineas = open(ruta_md, encoding="utf-8").read().split("\n")
    elementos = []
    i = 0

    while i < len(lineas):
        linea = lineas[i]

        # Bloques de codigo cercados.
        if linea.startswith("```"):
            j = i + 1
            bloque = []
            while j < len(lineas) and not lineas[j].startswith("```"):
                bloque.append(lineas[j])
                j += 1
            codigo = construir_codigo(bloque)
            # Un diagrama que cabe en una pagina no debe partirse a la mitad.
            elementos.append(KeepTogether(codigo) if len(bloque) <= 50 else codigo)
            elementos.append(Spacer(1, 8))
            i = j + 1
            continue

        # Tablas de tuberias.
        if linea.strip().startswith("|") and i + 1 < len(lineas) and es_separador_de_tabla(lineas[i + 1]):
            j = i
            filas = []
            while j < len(lineas) and lineas[j].strip().startswith("|"):
                filas.append(lineas[j])
                j += 1
            elementos.append(construir_tabla(filas))
            elementos.append(Spacer(1, 10))
            i = j
            continue

        # Reglas horizontales.
        if re.match(r"^\s*(-{3,}|\*{3,})\s*$", linea):
            elementos.append(Spacer(1, 4))
            elementos.append(HRFlowable(width="100%", thickness=0.6, color=colors.HexColor("#BFBFBF")))
            elementos.append(Spacer(1, 6))
            i += 1
            continue

        # Encabezados.
        encabezado = re.match(r"^(#{1,6})\s+(.*)$", linea)
        if encabezado:
            nivel = min(len(encabezado.group(1)), 4)
            elementos.append(Paragraph(a_marcado(encabezado.group(2)), ESTILOS["h%d" % nivel]))
            i += 1
            continue

        # Citas.
        if linea.startswith("> "):
            elementos.append(Paragraph(a_marcado(linea[2:]), ESTILOS["cita"]))
            i += 1
            continue

        # Listas.
        vineta = re.match(r"^(\s*)[-*+]\s+(.*)$", linea)
        numerada = re.match(r"^(\s*)\d+\.\s+(.*)$", linea)
        if vineta or numerada:
            numerado = numerada is not None
            patron = r"^(\s*)\d+\.\s+(.*)$" if numerado else r"^(\s*)[-*+]\s+(.*)$"
            items = []
            while i < len(lineas):
                m = re.match(patron, lineas[i])
                if not m:
                    break
                texto = m.group(2)
                i += 1
                # Lineas de continuacion sangradas.
                while (i < len(lineas) and lineas[i].startswith("  ") and lineas[i].strip()
                       and not re.match(r"^\s*([-*+]|\d+\.)\s", lineas[i])):
                    texto += " " + lineas[i].strip()
                    i += 1
                items.append(ListItem(Paragraph(a_marcado(texto), ESTILOS["lista"]), leftIndent=18))
            elementos.append(ListFlowable(items, bulletType="1" if numerado else "bullet",
                                          start="1" if numerado else None, leftIndent=16))
            elementos.append(Spacer(1, 6))
            continue

        # Linea en blanco.
        if not linea.strip():
            i += 1
            continue

        # Parrafo normal (se unen las lineas hasta el proximo elemento).
        bloque = [linea.strip()]
        i += 1
        while i < len(lineas) and lineas[i].strip() and not re.match(
            r"^(#{1,6}\s|```|\||\s*[-*+]\s|\s*\d+\.\s|>\s|-{3,}$)", lineas[i]
        ):
            bloque.append(lineas[i].strip())
            i += 1
        elementos.append(Paragraph(a_marcado(" ".join(bloque)), ESTILOS["cuerpo"]))

    def pie(canvas, documento):
        canvas.saveState()
        canvas.setFont(F_TEXTO, 7.5)
        canvas.setFillColor(colors.HexColor("#666666"))
        canvas.drawString(MARGEN, 0.45 * inch, normalizar("RECAUDO-T · Ping/Echo + Redundancia Activa"))
        canvas.drawRightString(letter[0] - MARGEN, 0.45 * inch, "Pagina %d" % documento.page)
        canvas.restoreState()

    documento = SimpleDocTemplate(ruta_pdf, pagesize=letter, leftMargin=MARGEN, rightMargin=MARGEN,
                                  topMargin=MARGEN, bottomMargin=0.7 * inch,
                                  title="Taller de disponibilidad - RECAUDO-T",
                                  author="Ingenieria de Software II")
    documento.build(elementos, onFirstPage=pie, onLaterPages=pie)
    print("generado %s (fuentes Unicode: %s)" % (ruta_pdf, "si" if HAY_UNICODE else "no"))


if __name__ == "__main__":
    entrada = sys.argv[1] if len(sys.argv) > 1 else "docs/documentacion.md"
    salida = sys.argv[2] if len(sys.argv) > 2 else "docs/documentacion.pdf"
    convertir(entrada, salida)
