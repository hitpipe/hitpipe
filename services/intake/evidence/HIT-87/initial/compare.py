import subprocess
for i in range(1,6):
 for version in (['baseline','optimized'] if i%2 else ['optimized','baseline']):
  subprocess.run(['python3','/tmp/hit87-run/capture.py',f'{version}-round-{i}',f'/tmp/hit87-run/{version}','-n','200'],check=True)
