import sys
import re

def remove_comments(filename):
    with open(filename, 'r') as f:
        content = f.read()
    
    # Remove // comments
    content = re.sub(r'//.*', '', content)
    
    # Remove /* ... */ comments
    content = re.sub(r'/*[\s\S]*?*/', '', content)
    
    # Remove empty lines
    content = "\n".join([line for line in content.split('\n') if line.strip()])

    with open(filename, 'w') as f:
        f.write(content)

if __name__ == '__main__':
    for filename in sys.argv[1:]:
        remove_comments(filename)
