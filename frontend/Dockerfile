FROM node:22

WORKDIR /app

COPY package*.json ./

# 💡 これだけでOK！全部の依存を解決（emoji-mart含む）
RUN npm install --legacy-peer-deps

COPY . .

EXPOSE 3000

CMD ["npm", "run", "dev"]
