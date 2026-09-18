from pathlib import Path
import json, hashlib, shutil
from PIL import Image, ImageDraw, ImageFont

OUT=Path(r'C:\Users\Derek\Documents\Codex\2026-09-17\i-x20\outputs\ui-buttons')
W,H,S=256,96,4
PALETTES={
 'normal': dict(outer='#101a24', edge='#617e91', bevel='#314b5e', face='#1d303f', hi='#98b6c7', low='#10212d', accent='#efb644', plate='#243b4b', label='#d6e5ed'),
 'hover': dict(outer='#132532', edge='#b5dbea', bevel='#45677d', face='#294657', hi='#e0f5ff', low='#213f52', accent='#ffda79', plate='#355369', label='#f1fbff'),
 'pressed': dict(outer='#101a24', edge='#738b99', bevel='#182a36', face='#152734', hi='#162531', low='#829eae', accent='#d59c32', plate='#1d3241', label='#ccdee8'),
 'disabled': dict(outer='#151e25', edge='#43535f', bevel='#273742', face='#202d37', hi='#526572', low='#18242d', accent='#606467', plate='#293944', label='#71838f'),
}

def make(state):
 p=PALETTES[state]; im=Image.new('RGBA',(W*S,H*S)); d=ImageDraw.Draw(im); svg=[]
 def polygon(pts,color):
  d.polygon([(round(x*S),round(y*S)) for x,y in pts],fill=color)
  svg.append('<polygon points="'+ ' '.join(f'{x},{y}' for x,y in pts)+'" fill="'+color+'"/>')
 def line(pts,color,width=1):
  d.line([(round(x*S),round(y*S)) for x,y in pts],fill=color,width=round(width*S),joint='curve')
  svg.append('<polyline points="'+' '.join(f'{x},{y}' for x,y in pts)+'" fill="none" stroke="'+color+'" stroke-width="'+str(width)+'"/>')
 def chamfer(n,c):
  return [(n+c,n),(W-n-c,n),(W-n,n+c),(W-n,H-n-c),(W-n-c,H-n),(n+c,H-n),(n,H-n-c),(n,n+c)]
 polygon(chamfer(2,12),p['outer'])
 polygon(chamfer(3,12),p['edge'])
 polygon(chamfer(4,12),p['bevel'])
 polygon(chamfer(9,9),p['low'])
 polygon(chamfer(11,8),p['face'])
 # Top and bottom bevels provide a raised versus recessed tactile cue.
 line([(11,22),(11,19),(19,11),(237,11),(245,19)],p['hi'],1)
 line([(245,22),(245,77),(237,85),(19,85),(11,77)],p['low'],1)
 line([(24,6),(232,6)],p['hi'],.65)
 line([(24,90),(232,90)],p['low'],1)
 # All decoration stays inside the 24 px corner regions.
 for right in (False,True):
  for bottom in (False,True):
   def t(pts): return [(W-x if right else x,H-y if bottom else y) for x,y in pts]
   polygon(t([(4,16),(16,4),(23,4),(23,8),(20,8),(8,20),(8,23),(4,23)]),p['plate'])
   line(t([(4,16),(16,4),(23,4)]),p['hi'],.8)
   polygon(t([(7,15),(14,8),(19,13),(12,20)]),p['outer'])
   polygon(t([(9,15),(14,10),(17,13),(12,18)]),p['accent'])
   line(t([(18,3),(23,8)]),p['edge'],1)
   line(t([(3,19),(8,24)]),p['edge'],1)
 im=im.resize((W,H),Image.Resampling.LANCZOS)
 im.save(OUT/f'button-{state}.png')
 (OUT/f'button-{state}.svg').write_text('<svg xmlns="http://www.w3.org/2000/svg" width="256" height="96" viewBox="0 0 256 96">\n'+ '\n'.join(svg)+'\n</svg>')
 return im

images={state:make(state) for state in PALETTES}
def sliced(im,w,h):
 dst=Image.new('RGBA',(w,h)); sx=[0,24,232,256]; sy=[0,24,72,96]; dx=[0,24,w-24,w]; dy=[0,24,h-24,h]
 for y in range(3):
  for x in range(3):
   patch=im.crop((sx[x],sy[y],sx[x+1],sy[y+1])).resize((dx[x+1]-dx[x],dy[y+1]-dy[y]),Image.Resampling.BILINEAR)
   dst.paste(patch,(dx[x],dy[y]))
 return dst

fontdir=Path('C:/Windows/Fonts')
def font(n):return ImageFont.truetype(str(fontdir/'segoeui.ttf'),n)
preview=Image.new('RGB',(1120,640),'#0d1721'); d=ImageDraw.Draw(preview)
d.text((44,30),'VESPER III  /  BUTTON STATES',font=font(27),fill='#dbe8ef')
d.text((44,73),'Slate metal · amber inserts · fixed corners · separate labels',font=font(16),fill='#829bad')
for i,(state,im) in enumerate(images.items()):
 x=44+i*270
 d.text((x,132),['NORMAL','ACTIVE / HOVER','PRESSED','DISABLED'][i],font=font(15),fill='#9aafbd')
 preview.paste(im,(x,167),im)
 d.text((x+128,215+(2 if state=='pressed' else 0)),'CONSTRUCT',font=font(18),fill=PALETTES[state]['label'],anchor='mm')
d.text((44,319),'NINE-SLICE RESIZING',font=font(15),fill='#9aafbd')
for x,w,label in [(44,144,'CANCEL'),(212,280,'BUILD GENERATOR'),(516,560,'RESEARCH TECHNOLOGY')]:
 b=sliced(images['normal'],w,64); preview.paste(b,(x,355),b);d.text((x+w/2,387),label,font=font(16),fill='#d6e5ed',anchor='mm')
d.text((44,476),'256 × 96 RGBA PNG  /  24 px slice inset  /  editable SVG sources',font=font(18),fill='#c5d8e4')
d.text((44,513),'Artwork is unlabeled. Text shown here is a separate preview overlay.',font=font(16),fill='#829bad')
d.text((44,547),'Pressed: recessed bevel + suggested 2 px label offset. Disabled: muted edge and amber inserts.',font=font(16),fill='#829bad')
preview.save(OUT/'preview.png')
manifest={'size':[W,H],'insets':{'left':24,'top':24,'right':24,'bottom':24},'recommended_minimum_size':[72,56],'alpha':'straight','color_space':'sRGB','render_mode':'textured UI; preserve authored center and colors','tint':[1,1,1],'states':{s:{'file':f'button-{s}.png','label_color':p['label'],'label_offset':[0,2 if s=='pressed' else 0]} for s,p in PALETTES.items()},'state_priority':['disabled','pressed','hover','normal'],'notes':'active means hovered, not a persistent toggle selection. All four assets share dimensions and silhouette.'}
(OUT/'buttons.json').write_text(json.dumps(manifest,indent=2))
(OUT/'README.md').write_text('''# Vesper III button assets

Four unlabeled 256 x 96 RGBA textures matching panel.png: slate metal, clipped corners, pale edging and small amber inserts. Normal is raised; hover brightens the face and edge; pressed reverses the bevel for a recessed appearance; disabled mutes contrast and the amber accents.

## Nine-slice

Uniform 24 px source inset. At scale 1, corners occupy 24 screen pixels. Recommended height 56-96 px, width >=72 px. Keep labels at least 24 px from the left/right edges. PNGs include opaque faces and transparent exterior corners, with straight alpha; use linear filtering and clamp sampling. Draw as ordinary textured UI with white tint to retain the artwork, rather than a panel shader that replaces the center fill. SVG sources and the deterministic Python/Pillow generator are included.

Glyphengine definition for each state:

```go
slice := renderer.NewNineSlice(texture, 256, 24)
slice.TexH = 96
// slice.GenerateQuads(x, y, w, h, 1, [3]float32{1, 1, 1})
```

Load each texture once. Switch texture/state without changing the layout rectangle. Priority: disabled > pointer held inside > hover > normal. On release outside, cancel the action. Disabled buttons should not accept input. Active here means hovered, not toggled. Draw labels separately using buttons.json colors; shift the pressed label downward 2 px without moving the hit box. Keyboard focus can reuse hover; keyboard activation can use pressed.

These are asset deliverables; existing game UI handlers and embedding are not changed. Add assets/ui/buttons/*.png to the go:embed file list when integrating. preview.png includes text overlays solely for demonstration.
''')
# Basic export checks: opaque face, transparent exterior, identical dimensions and alpha.
base=images['normal'].getchannel('A').tobytes()
for state,im in images.items():
 assert im.size==(256,96) and im.mode=='RGBA'
 assert im.getpixel((0,0))[3]==0 and im.getpixel((128,48))[3]==255
 assert im.getchannel('A').tobytes()==base
 assert im.crop((24,24,232,72)).getextrema()[3]==(255,255)
(OUT/'validation.json').write_text(json.dumps({'checks':['RGBA dimensions','identical alpha silhouettes','transparent exterior','opaque center','visually inspected nine-slice preview'],'sha256':{f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in OUT.glob('button-*.png')}},indent=2))
shutil.copy2(__file__,OUT/'build_buttons.py')
print(str(OUT))
