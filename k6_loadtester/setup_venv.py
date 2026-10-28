#!/usr/bin/env python3
"""
Setup script to create and activate a virtual environment for the k6 load tester.
Run this once to set up the environment, or use the activate script.
"""

import os
import sys
import subprocess
import venv
from pathlib import Path

PROJECT_DIR = Path(__file__).parent.resolve()
VENV_DIR = PROJECT_DIR / "venv"

def create_venv():
    """Create a virtual environment in the venv directory."""
    print(f"Creating virtual environment in {VENV_DIR}...")
    venv.create(VENV_DIR, with_pip=True)
    print("Virtual environment created.")

def install_requirements():
    """Install requirements from requirements.txt."""
    req_file = PROJECT_DIR / "requirements.txt"
    if req_file.exists():
        print(f"Installing requirements from {req_file}...")
        
        # Determine the pip executable
        pip_executable = VENV_DIR / "bin" / "pip"
        if sys.platform == "win32":
            pip_executable = VENV_DIR / "Scripts" / "pip.exe"
        
        subprocess.run([str(pip_executable), "install", "-r", str(req_file)], check=True)
        print("Requirements installed.")
    else:
        print(f"Warning: {req_file} not found. No requirements to install.")

def create_activate_script():
    """Create a convenient activate script."""
    activate_script = PROJECT_DIR / "activate"
    
    if sys.platform == "win32":
        activate_content = f"""@echo off
call {VENV_DIR}\\Scripts\\activate
python {PROJECT_DIR}/loadtester.py %*
"""
    else:
        activate_content = f"""#!/bin/bash
source {VENV_DIR}/bin/activate
python3 {PROJECT_DIR}/loadtester.py "$@"
"""
    
    with open(activate_script, "w") as f:
        f.write(activate_content)
    
    if sys.platform != "win32":
        os.chmod(activate_script, 0o755)
    
    print(f"Created activate script at {activate_script}")

def main():
    print("Setting up k6 load tester virtual environment...")
    print(f"Project directory: {PROJECT_DIR}")
    
    # Check if venv already exists
    if VENV_DIR.exists():
        print(f"Virtual environment already exists at {VENV_DIR}")
        print("To re-create it, first delete the 'venv' directory.")
        return
    
    create_venv()
    install_requirements()
    create_activate_script()
    
    print("\nSetup complete!")
    print(f"To run the load tester: ./activate")
    print(f"Or manually: source {VENV_DIR}/bin/activate && python3 {PROJECT_DIR}/loadtester.py")

if __name__ == "__main__":
    main()
