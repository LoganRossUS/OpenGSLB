# Configuration file for the Sphinx documentation builder.
# https://www.sphinx-doc.org/en/master/usage/configuration.html

# -- Project information -----------------------------------------------------
project = 'OpenGSLB'
copyright = '2025, Logan Ross'
author = 'Logan Ross'

# -- General configuration ---------------------------------------------------
extensions = [
    'myst_parser',
    'sphinx.ext.autodoc',
    'sphinx.ext.viewcode',
    'sphinx_copybutton',
    'sphinx_design',
]

# MyST Parser configuration for Markdown support
myst_enable_extensions = [
    'colon_fence',
    'deflist',
    'fieldlist',
    'html_admonition',
    'html_image',
    'replacements',
    'smartquotes',
    'strikethrough',
    'substitution',
    'tasklist',
]

# Allow MyST to generate anchors for headings
myst_heading_anchors = 3

# Source file suffixes
source_suffix = {
    '.rst': 'restructuredtext',
    '.md': 'markdown',
}

# The master toctree document
master_doc = 'index'

# Exclude patterns
exclude_patterns = ['_build', 'Thumbs.db', '.DS_Store']

# -- Options for HTML output -------------------------------------------------
html_theme = 'sphinx_rtd_theme'

html_theme_options = {
    'navigation_depth': 4,
    'collapse_navigation': False,
    'sticky_navigation': True,
    'includehidden': True,
    'titles_only': False,
}

# Static files (uncomment and create _static dir if you add custom CSS/JS)
# html_static_path = ['_static']

# -- Options for linkcheck ---------------------------------------------------
linkcheck_ignore = [
    r'http://localhost.*',
    r'http://127\.0\.0\.1.*',
]

# Suppress warnings for missing references in markdown files
suppress_warnings = ['myst.header', 'ref.myst']
