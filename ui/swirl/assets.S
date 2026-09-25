/* SWIRL: fonts baked by swirl/tools/gen_fonts.py, linked into the binary so no extra files are needed on the SD card */
    .section .rodata
    .balign 32
    .global _swirl_font_title
_swirl_font_title:
    .incbin "ui/swirl/assets/font_title.swf"
    .balign 32
    .global _swirl_font_head
_swirl_font_head:
    .incbin "ui/swirl/assets/font_head.swf"
    .balign 32
    .global _swirl_font_ui
_swirl_font_ui:
    .incbin "ui/swirl/assets/font_ui.swf"
    .balign 32
    .global _swirl_font_body
_swirl_font_body:
    .incbin "ui/swirl/assets/font_body.swf"
    .balign 32
    .global _swirl_font_small
_swirl_font_small:
    .incbin "ui/swirl/assets/font_small.swf"
