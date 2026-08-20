import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig({plugins:[react()],build:{outDir:"../site/web",emptyOutDir:true},server:{port:5174,proxy:{"/api":"http://127.0.0.1:8080","/livez":"http://127.0.0.1:8080","/readyz":"http://127.0.0.1:8080"}}});
