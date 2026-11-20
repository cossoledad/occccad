from conan import ConanFile
from conan.tools.files import copy, patch
from pathlib import Path


class OccccadOpenCascade(ConanFile):
    name = "opencascade"
    version = "7.9.1"
    user = "occccad"
    channel = "stable"
    python_requires = "opencascade/7.9.1#c5fddcb0f309bc0855a6394e0c7e377a"
    python_requires_extend = "opencascade.OpenCascadeConan"

    patches = ("shell-face-edge-lookup.patch", "mesh-wire-bounds.patch")

    def export_sources(self):
        for filename in self.patches:
            copy(self, filename, src=Path(self.recipe_folder).parent / "patches/opencascade/7.9.1",
                 dst=self.export_sources_folder)

    def source(self):
        super().source()
        for filename in self.patches:
            patch(self, patch_file=str(Path(self.export_sources_folder) / filename),
                  base_path=self.source_folder)
