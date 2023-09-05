PREFIX ?= /usr/local
MANDIR ?= $(PREFIX)/share/man

ASCIIDOC := asciidoctor

FOOTER := footer.adoc

SRC1 += checkpointctl-inspect.adoc
SRC1 += checkpointctl-memparse.adoc
SRC1 += checkpointctl-show.adoc
SRC1 += checkpointctl.adoc
SRC := $(SRC1)

MAN1S := $(patsubst %.adoc,%.1,$(SRC1))
MANS := $(MAN1S)
MAN1DIR := $(MANDIR)/man1

all: check $(MANS)

check:
	@$(ASCIIDOC) --version > /dev/null || exit 1

include ../Makefile.versions
$(FOOTER):
	$(call msg-gen, $@)
	@echo "The CRIU team" >  $@
	@echo "v$(VERSION)" >>  $@
	@echo ":doctype: manpage" >>  $@
	@echo ":mansource: checkpointctl" >> $@
	@echo ":manversion: $(VERSION)" >> $@
	@echo ":manmanual: CHECKPOINTCTL Manual" >> $@

%.1: %.adoc $(FOOTER) custom.xsl
	$(call msg-gen, $@)
	@$(ASCIIDOC) -b manpage -d manpage -o $@ $<

clean:
	@rm -f $(MANS) $(FOOTER)

install: check $(MANS)
	@echo "  INSTALL " $(MAN1S)
	@mkdir -p $(DESTDIR)$(MAN1DIR)
	@install -m 644 $(MAN1S) $(DESTDIR)$(MAN1DIR)

uninstall:
	@echo " UNINSTALL" $(MAN1S)
	@rm -rf $(addprefix $(DESTDIR)$(MAN1DIR)/,$(MAN1S))

.PHONY: all check clean install uninstall
