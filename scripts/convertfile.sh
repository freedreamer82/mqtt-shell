
FILE="$1"
OUTPUT="$2"

VALUE=$(base64 "$FILE"  | tr -d \\n)
#echo $VALUE

echo "To write in mqtt-shell output-path-file the command is :"
echo  
echo "echo \"${VALUE}\" | base64 --decode  > "output-path-file""
echo